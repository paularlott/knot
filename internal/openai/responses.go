package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/knot/internal/log"
)

const (
	// DefaultResponseWorkerPoolSize is the default size of the response worker pool
	DefaultResponseWorkerPoolSize = 10
	// DefaultResponseTTL is the default TTL for responses
	DefaultResponseTTL = 30 * 24 * time.Hour // 30 days
)

var (
	globalResponseWorkerPool *ResponseWorkerPool
	globalGossipFunc         GossipCallback
)

// InitResponseWorker initializes the global response worker pool
func InitResponseWorker(client *Client, poolSize int, gossipFunc GossipCallback) {
	if globalResponseWorkerPool != nil {
		return
	}
	globalGossipFunc = gossipFunc
	globalResponseWorkerPool = NewResponseWorkerPool(client, poolSize, gossipFunc)
	globalResponseWorkerPool.Start()

	// Recover incomplete responses on startup
	go recoverIncompleteResponses(client)
}

// ShutdownResponseWorker gracefully shuts down the response worker pool
func ShutdownResponseWorker() {
	if globalResponseWorkerPool != nil {
		globalResponseWorkerPool.Stop()
		globalResponseWorkerPool = nil
	}
}

// EnqueueResponse enqueues a response for async processing and gossips it to the cluster
// This is used by non-HTTP paths (like MCP) to trigger response processing
func EnqueueResponse(response *model.Response) {
	if globalResponseWorkerPool == nil {
		log.Error("Response worker pool not initialized, cannot enqueue response", "id", response.Id)
		return
	}

	// Gossip the response (pending status) so all cluster nodes are aware
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	// Enqueue for processing
	globalResponseWorkerPool.Enqueue(response)
	log.Debug("Response enqueued for async processing", "id", response.Id)
}

// CancelResponse cancels an in-progress response in the worker pool and gossips the cancellation
// This is used by non-HTTP paths (like MCP) to cancel response processing
func CancelResponse(response *model.Response) {
	if globalResponseWorkerPool != nil {
		// Cancel the context in the worker pool
		globalResponseWorkerPool.Cancel(response.Id)
	}

	// Gossip the cancellation so all cluster nodes are aware
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}
}

// ProcessResponseSynchronously processes a response synchronously and returns the result
// This is used by non-HTTP paths (like MCP) that need synchronous processing
func ProcessResponseSynchronously(ctx context.Context, client *Client, response *model.Response) (*model.Response, error) {
	db := database.GetInstance()

	// Update status to in_progress
	response.Status = model.StatusInProgress
	response.UpdatedAt = hlc.Now()
	if err := db.SaveResponse(response); err != nil {
		return nil, fmt.Errorf("failed to update response status to in_progress: %w", err)
	}

	// Gossip the in_progress status
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	// Process the response directly
	processor := &responseProcessor{client: client}
	result, err := processor.Process(ctx, response)

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
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	return response, nil
}

// recoverIncompleteResponses finds and re-queues incomplete responses on startup
func recoverIncompleteResponses(client *Client) {
	db := database.GetInstance()

	// Get in_progress and pending responses
	inProgressResponses, err := db.GetResponsesByStatus(model.StatusInProgress)
	if err != nil {
		log.WithError(err).Error("Failed to get in_progress responses for recovery")
		return
	}

	pendingResponses, err := db.GetResponsesByStatus(model.StatusPending)
	if err != nil {
		log.WithError(err).Error("Failed to get pending responses for recovery")
		return
	}

	// Combine and reprocess
	all := append(inProgressResponses, pendingResponses...)
	for _, resp := range all {
		// Check TTL - only reprocess if not expired
		if resp.ExpiresAt == nil || resp.ExpiresAt.After(time.Now().UTC()) {
			log.Info("Recovering incomplete response", "id", resp.Id, "status", resp.Status)
			globalResponseWorkerPool.Enqueue(resp)
		} else {
			log.Info("Skipping expired response during recovery", "id", resp.Id)
		}
	}

	if len(all) > 0 {
		log.Info("Recovered incomplete responses", "count", len(all))
	}
}

// Handles POST /v1/responses
func (s *Service) HandleCreateResponse(w http.ResponseWriter, r *http.Request) {
	var ctx context.Context = r.Context()

	var req CreateResponseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WithError(err).Error("OpenAI: Failed to decode response request")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Get user from context
	userI := ctx.Value("user")
	if userI == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, map[string]string{
			"error": "Unauthorized",
		})
		return
	}
	user := userI.(*model.User)

	// Validate request
	if req.Model == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "model is required",
		})
		return
	}

	// Create response object
	response := model.NewResponse(user.Id, "", DefaultResponseTTL)
	response.Status = model.StatusPending

	// Store the request
	if err := response.SetRequest(req); err != nil {
		log.WithError(err).Error("OpenAI: Failed to store request")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to store request",
		})
		return
	}

	// Save to database
	db := database.GetInstance()
	if err := db.SaveResponse(response); err != nil {
		log.WithError(err).Error("OpenAI: Failed to save response")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to save response",
		})
		return
	}

	// Check if background processing is requested (default is false = synchronous)
	background := req.Background

	if background {
		// Asynchronous: gossip, enqueue, and return immediately with pending status
		if globalGossipFunc != nil {
			globalGossipFunc(response)
		}
		globalResponseWorkerPool.Enqueue(response)

		rest.WriteResponse(http.StatusAccepted, w, r, ResponseObject{
			ID:        response.Id,
			Object:    "response",
			Status:    string(response.Status),
			CreatedAt: response.CreatedAt.Unix(),
			Output:    []interface{}{}, // Always initialize as empty array for N8N compatibility
			Tools:     []Tool{},        // Always initialize as empty array
		})
		return
	}

	// Synchronous: process directly in this HTTP handler (bypass queue)
	log.Info("Processing response synchronously", "id", response.Id)

	// Update status to in_progress
	response.Status = model.StatusInProgress
	response.UpdatedAt = hlc.Now()
	if err := db.SaveResponse(response); err != nil {
		log.WithError(err).Error("Failed to update response status to in_progress", "id", response.Id)
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to update response status",
		})
		return
	}

	// Gossip the in_progress status
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	// Process the response directly
	processor := &responseProcessor{client: s.client}
	result, err := processor.Process(ctx, response)

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
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	// Return the completed response
	responseObj := s.convertToResponseObject(response)
	rest.WriteResponse(http.StatusOK, w, r, responseObj)
}

// Handles GET /v1/responses/{response_id}
func (s *Service) HandleGetResponse(w http.ResponseWriter, r *http.Request) {
	responseId := r.PathValue("response_id")
	if !validate.UUID(responseId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid response ID",
		})
		return
	}

	db := database.GetInstance()
	response, err := db.GetResponse(responseId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{
			"error": "Response not found",
		})
		return
	}

	// Convert to ResponseObject format
	responseObj := s.convertToResponseObject(response)
	rest.WriteResponse(http.StatusOK, w, r, responseObj)
}

// Handles DELETE /v1/responses/{response_id}
func (s *Service) HandleDeleteResponse(w http.ResponseWriter, r *http.Request) {
	responseId := r.PathValue("response_id")
	if !validate.UUID(responseId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid response ID",
		})
		return
	}

	db := database.GetInstance()
	response, err := db.GetResponse(responseId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{
			"error": "Response not found",
		})
		return
	}

	// Soft delete with 7-day grace period before final deletion
	response.IsDeleted = true
	response.UpdatedAt = hlc.Now()
	// Set ExpiresAt to 7 days from now for grace period
	gracePeriod := 7 * 24 * time.Hour
	expiresAt := time.Now().UTC().Add(gracePeriod)
	response.ExpiresAt = &expiresAt

	if err := db.SaveResponse(response); err != nil {
		log.WithError(err).Error("OpenAI: Failed to delete response")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to delete response",
		})
		return
	}

	// Gossip the deletion
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	w.WriteHeader(http.StatusOK)
}

// Handles POST /v1/responses/{response_id}/cancel
func (s *Service) HandleCancelResponse(w http.ResponseWriter, r *http.Request) {
	responseId := r.PathValue("response_id")
	if !validate.UUID(responseId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid response ID",
		})
		return
	}

	db := database.GetInstance()
	response, err := db.GetResponse(responseId)
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, map[string]string{
			"error": "Response not found",
		})
		return
	}

	// Can only cancel pending or in_progress responses
	if response.Status != model.StatusPending && response.Status != model.StatusInProgress {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": fmt.Sprintf("Cannot cancel response with status %s", response.Status),
		})
		return
	}

	// Update status to cancelled
	response.Status = model.StatusCancelled
	response.UpdatedAt = hlc.Now()
	if err := db.SaveResponse(response); err != nil {
		log.WithError(err).Error("OpenAI: Failed to cancel response")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Failed to cancel response",
		})
		return
	}

	// Cancel the context in the worker pool
	globalResponseWorkerPool.Cancel(responseId)

	// Gossip the cancellation
	if globalGossipFunc != nil {
		globalGossipFunc(response)
	}

	// Return the updated response
	responseObj := s.convertToResponseObject(response)
	rest.WriteResponse(http.StatusOK, w, r, responseObj)
}

// convertToResponseObject converts a database Response to OpenAI ResponseObject format
func (s *Service) convertToResponseObject(response *model.Response) ResponseObject {
	obj := ResponseObject{
		ID:        response.Id,
		Object:    "response",
		CreatedAt: response.CreatedAt.Unix(),
		Status:    string(response.Status),
		Output:    []interface{}{}, // Always initialize as empty array for N8N compatibility
		Tools:     []Tool{},        // Always initialize as empty array
	}

	// Add error if failed
	if response.Status == model.StatusFailed && response.Error != "" {
		obj.Error = &APIError{
			Message: response.Error,
		}
	}

	// Add request data
	var req CreateResponseRequest
	if err := response.GetRequest(&req); err == nil {
		obj.Model = req.Model
		obj.PreviousResponseID = req.PreviousResponseID
		if len(req.Tools) > 0 {
			obj.Tools = req.Tools
		}
		obj.Metadata = req.Metadata
		// Add other fields as needed
	}

	// Add response data if completed
	if response.Status == model.StatusCompleted {
		var respData map[string]interface{}
		if err := response.GetResponse(&respData); err == nil {
			// Extract output, usage, etc.
			if output, ok := respData["output"].([]interface{}); ok {
				obj.Output = output
			}
			if usage, ok := respData["usage"].(*Usage); ok {
				obj.Usage = usage
			}
		}
	}

	return obj
}
