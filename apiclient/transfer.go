package apiclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// TransferIdleTimeout is how long a transfer of file content, either way,
// may go without moving any data before it is given up. A transfer has no
// limit on how long it takes in all, so a large file is never cut off part
// way; one that has stalled (a connection that stopped moving data without
// closing) fails after this instead of waiting on TCP to notice.
var TransferIdleTimeout = 2 * time.Minute

// ErrTransferStalled is the error of a transfer given up on as stalled.
var ErrTransferStalled = errors.New("the transfer stalled")

// doTransfer sends a request that moves file content, given up on when no
// data moves for TransferIdleTimeout: while the body is sent, and while the
// response is read. Between the two, while the server takes in what was
// sent, it waits as long as it takes. The caller closes the response body.
func (c *ApiClient) doTransfer(ctx context.Context, method, path string, body io.Reader, size int64, headers map[string]string) (*http.Response, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	idle := TransferIdleTimeout
	ctx, cancel := context.WithCancelCause(ctx)
	w := &idleWatch{idle: idle, cancel: cancel}
	w.start()
	if body != nil {
		body = &idleReader{r: body, w: w, stopAtEOF: true}
	}
	resp, err := hc.DoRaw(ctx, method, path, body, size, headers)
	if err != nil {
		w.stop()
		if cause := context.Cause(ctx); errors.Is(cause, ErrTransferStalled) {
			cancel(nil)
			return nil, cause
		}
		cancel(nil)
		return nil, err
	}
	w.start()
	resp.Body = &idleBody{idleReader: idleReader{r: resp.Body, w: w}, body: resp.Body, ctx: ctx, cancel: cancel}
	return resp, nil
}

// idleWatch gives a transfer up when it is not touched for idle.
type idleWatch struct {
	mu     sync.Mutex
	idle   time.Duration
	timer  *time.Timer
	cancel context.CancelCauseFunc
}

func (w *idleWatch) start() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(w.idle, func() {
		w.cancel(fmt.Errorf("%w: no data moved for %s", ErrTransferStalled, w.idle))
	})
}

func (w *idleWatch) touch() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Reset(w.idle)
	}
}

func (w *idleWatch) stop() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
}

// idleReader touches the watch whenever data moves.
type idleReader struct {
	r         io.Reader
	w         *idleWatch
	stopAtEOF bool // a body sent: once it is all sent the server is waited for
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.w.touch()
	}
	if err == io.EOF && r.stopAtEOF {
		r.w.stop()
	}
	return n, err
}

// idleBody is a response body read under the watch; closing it ends the
// transfer.
type idleBody struct {
	idleReader
	body   io.Closer
	ctx    context.Context
	cancel context.CancelCauseFunc
}

func (b *idleBody) Read(p []byte) (int, error) {
	n, err := b.idleReader.Read(p)
	if err != nil && err != io.EOF {
		if cause := context.Cause(b.ctx); errors.Is(cause, ErrTransferStalled) {
			err = cause
		}
	}
	return n, err
}

func (b *idleBody) Close() error {
	b.w.stop()
	err := b.body.Close()
	b.cancel(nil)
	return err
}
