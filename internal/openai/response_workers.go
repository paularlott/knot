package openai

import (
	"context"
	"sync"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
)

// GossipCallback is a function type for gossiping response updates
type GossipCallback func(response *model.Response)

// ResponseWorkerPool handles async response processing
type ResponseWorkerPool struct {
	client       *Client
	queue        chan *model.Response
	ctx          context.Context
	cancel       context.CancelFunc
	cancelReqs   map[string]context.CancelFunc
	cancelMux    sync.Mutex
	wg           sync.WaitGroup
	gossipFunc   GossipCallback
}

// NewResponseWorkerPool creates a new response worker pool
func NewResponseWorkerPool(client *Client, size int, gossipFunc GossipCallback) *ResponseWorkerPool {
	if size <= 0 {
		size = DefaultResponseWorkerPoolSize
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ResponseWorkerPool{
		client:     client,
		queue:      make(chan *model.Response, size*2), // Buffer for queued responses
		ctx:        ctx,
		cancel:     cancel,
		cancelReqs: make(map[string]context.CancelFunc),
		gossipFunc: gossipFunc,
	}
}

// Start starts the worker pool
func (p *ResponseWorkerPool) Start() {
	for i := 0; i < DefaultResponseWorkerPoolSize; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
	log.Info("Response worker pool started", "workers", DefaultResponseWorkerPoolSize)
}

// Stop stops the worker pool gracefully
func (p *ResponseWorkerPool) Stop() {
	log.Info("Stopping response worker pool...")
	p.cancel() // Cancel all in-flight requests
	close(p.queue)
	p.wg.Wait()
	log.Info("Response worker pool stopped")
}

// Enqueue adds a response to the processing queue
func (p *ResponseWorkerPool) Enqueue(response *model.Response) {
	select {
	case p.queue <- response:
		log.Debug("Response enqueued for processing", "id", response.Id)
	default:
		log.Error("Response queue full, dropping response", "id", response.Id)
	}
}

// Cancel cancels an in-progress response
func (p *ResponseWorkerPool) Cancel(responseId string) {
	p.cancelMux.Lock()
	defer p.cancelMux.Unlock()

	if cancel, ok := p.cancelReqs[responseId]; ok {
		cancel()
		delete(p.cancelReqs, responseId)
		log.Debug("Cancelled response", "id", responseId)
	}
}

// worker processes responses from the queue
func (p *ResponseWorkerPool) worker(id int) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case response, ok := <-p.queue:
			if !ok {
				return
			}

			log.Debug("Worker processing response", "worker", id, "id", response.Id)

			// Check if already cancelled
			db := database.GetInstance()
			current, err := db.GetResponse(response.Id)
			if err != nil || current.Status == model.StatusCancelled {
				continue
			}

			// Process the response
			p.processResponse(response)
		}
	}
}

// processResponse processes a single response
func (p *ResponseWorkerPool) processResponse(response *model.Response) {
	db := database.GetInstance()

	// Update status to in_progress
	response.Status = model.StatusInProgress
	response.UpdatedAt = hlc.Now()
	if err := db.SaveResponse(response); err != nil {
		log.WithError(err).Error("Failed to update response status to in_progress", "id", response.Id)
		return
	}

	// Gossip the status update
	if p.gossipFunc != nil {
		p.gossipFunc(response)
	}

	// Create cancelable context for this response
	reqCtx, cancel := context.WithCancel(p.ctx)
	p.cancelMux.Lock()
	p.cancelReqs[response.Id] = cancel
	p.cancelMux.Unlock()
	defer func() {
		p.cancelMux.Lock()
		delete(p.cancelReqs, response.Id)
		p.cancelMux.Unlock()
	}()

	// Process the response
	processor := &responseProcessor{client: p.client}
	result, err := processor.Process(reqCtx, response)

	// Update response with result
	response.UpdatedAt = hlc.Now()
	if err != nil {
		response.Status = model.StatusFailed
		response.Error = err.Error()
		log.WithError(err).Error("Response processing failed", "id", response.Id)
	} else {
		response.Status = model.StatusCompleted
		if err := response.SetResponse(result); err != nil {
			log.WithError(err).Error("Failed to store response result", "id", response.Id)
		}
	}

	if saveErr := db.SaveResponse(response); saveErr != nil {
		log.WithError(saveErr).Error("Failed to save final response status", "id", response.Id)
	}

	// Gossip the final result
	if p.gossipFunc != nil {
		p.gossipFunc(response)
	}

	log.Debug("Response processing complete", "id", response.Id, "status", response.Status)
}
