package rest

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
)

var (
	errUnsupportedContentType = errors.New("Content-Type header is not application/json or application/msgpack")
	errInvalidStatusCode      = errors.New("invalid status code")
)

// DecodeRequestBody decodes JSON or Msgpack data from the request body into a struct
func DecodeRequestBody(w http.ResponseWriter, r *http.Request, v interface{}) error {
	if r == nil || v == nil {
		http.Error(w, "invalid request or destination", http.StatusBadRequest)
		return errors.New("invalid request or destination")
	}

	contentType := r.Header.Get("Content-Type")
	// Handle possible charset in Content-Type
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	switch contentType {
	case "application/json":
		decoder := json.NewDecoder(r.Body)
		// decoder.DisallowUnknownFields()
		if err := decoder.Decode(v); err != nil {
			http.Error(w, "invalid JSON format", http.StatusBadRequest)
			return err
		}
		return nil
	case "application/msgpack":
		decoder := msgpack.NewDecoder(r.Body)
		if err := decoder.Decode(v); err != nil {
			http.Error(w, "invalid msgpack format", http.StatusBadRequest)
			return err
		}
		return nil
	default:
		http.Error(w, errUnsupportedContentType.Error(), http.StatusUnsupportedMediaType)
		return errUnsupportedContentType
	}
}

// WriteResponse encodes and writes a JSON or Msgpack response
func WriteResponse(status int, w http.ResponseWriter, r *http.Request, v interface{}) error {
	if w == nil || r == nil {
		return errors.New("invalid response writer or request")
	}

	if status < 100 || status > 599 {
		http.Error(w, errInvalidStatusCode.Error(), http.StatusInternalServerError)
		return errInvalidStatusCode
	}

	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/msgpack") {
		w.Header().Set("Content-Type", "application/msgpack")
		w.WriteHeader(status)
		return msgpack.NewEncoder(w).Encode(v)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(v)
}

// StreamWriter writes chunks using Server-Sent Events
type StreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

// NewStreamWriter creates a new stream writer
func NewStreamWriter(w http.ResponseWriter, r *http.Request) *StreamWriter {
	if w == nil || r == nil {
		return nil
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	flusher, _ := w.(http.Flusher)

	return &StreamWriter{
		w:       w,
		flusher: flusher,
		closed:  false,
	}
}

// WriteChunk writes a single chunk as SSE
func (sw *StreamWriter) WriteChunk(chunk interface{}) error {
	if sw.closed {
		return errors.New("stream writer is closed")
	}

	// SSE always uses JSON
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("failed to marshal chunk: %w", err)
	}

	if _, err := fmt.Fprintf(sw.w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	if sw.flusher != nil {
		sw.flusher.Flush()
	}

	return nil
}

// WriteEnd signals the end of the stream
func (sw *StreamWriter) WriteEnd() error {
	if sw.closed {
		return nil
	}

	if _, err := fmt.Fprint(sw.w, "data: [DONE]\n\n"); err != nil {
		return fmt.Errorf("failed to write end marker: %w", err)
	}

	if sw.flusher != nil {
		sw.flusher.Flush()
	}

	return nil
}

// Close closes the stream writer
func (sw *StreamWriter) Close() error {
	sw.closed = true
	return nil
}
