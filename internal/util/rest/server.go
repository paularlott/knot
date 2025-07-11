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
			http.Error(w, err.Error(), http.StatusBadRequest)
			return err
		}
		return nil
	case "application/msgpack":
		decoder := msgpack.NewDecoder(r.Body)
		// decoder.DisallowUnknownFields(true)
		if err := decoder.Decode(v); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
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

// Add these types to existing server.go
type StreamType int

const (
	StreamChunked StreamType = iota
	StreamSSE
)

// StreamWriter interface for different streaming types
type StreamWriter interface {
	WriteChunk(chunk interface{}) error
	WriteEnd() error
	Close() error
}

// ChunkedStreamWriter writes chunks using Transfer-Encoding: chunked
type ChunkedStreamWriter struct {
	w          http.ResponseWriter
	flusher    http.Flusher
	useMsgPack bool
	closed     bool
}

// NewChunkedStreamWriter creates a new chunked stream writer
func NewChunkedStreamWriter(w http.ResponseWriter, r *http.Request) *ChunkedStreamWriter {
	// Determine encoding based on Accept header
	accept := r.Header.Get("Accept")
	useMsgPack := strings.Contains(accept, "application/msgpack")

	if useMsgPack {
		w.Header().Set("Content-Type", "application/msgpack")
	} else {
		w.Header().Set("Content-Type", "application/x-ndjson")
	}

	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, _ := w.(http.Flusher)

	return &ChunkedStreamWriter{
		w:          w,
		flusher:    flusher,
		useMsgPack: useMsgPack,
		closed:     false,
	}
}

// WriteChunk writes a single chunk
func (csw *ChunkedStreamWriter) WriteChunk(chunk interface{}) error {
	if csw.closed {
		return errors.New("stream writer is closed")
	}

	var data []byte
	var err error

	if csw.useMsgPack {
		data, err = msgpack.Marshal(chunk)
	} else {
		data, err = json.Marshal(chunk)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal chunk: %w", err)
	}

	// For chunked encoding, write the data directly
	if _, err := csw.w.Write(data); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	// Add newline for JSON readability (msgpack is binary)
	if !csw.useMsgPack {
		if _, err := csw.w.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write newline: %w", err)
		}
	}

	if csw.flusher != nil {
		csw.flusher.Flush()
	}

	return nil
}

// WriteEnd signals the end of the stream
func (csw *ChunkedStreamWriter) WriteEnd() error {
	if csw.closed {
		return nil
	}

	// For chunked encoding, we just stop writing
	// The connection will naturally close
	return nil
}

// Close closes the stream writer
func (csw *ChunkedStreamWriter) Close() error {
	csw.closed = true
	return nil
}

// SSEStreamWriter writes chunks using Server-Sent Events
type SSEStreamWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	closed  bool
}

// NewSSEStreamWriter creates a new SSE stream writer
func NewSSEStreamWriter(w http.ResponseWriter, r *http.Request) *SSEStreamWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	flusher, _ := w.(http.Flusher)

	return &SSEStreamWriter{
		w:       w,
		flusher: flusher,
		closed:  false,
	}
}

// WriteChunk writes a single chunk as SSE
func (ssw *SSEStreamWriter) WriteChunk(chunk interface{}) error {
	if ssw.closed {
		return errors.New("stream writer is closed")
	}

	// SSE always uses JSON
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("failed to marshal chunk: %w", err)
	}

	if _, err := fmt.Fprintf(ssw.w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	if ssw.flusher != nil {
		ssw.flusher.Flush()
	}

	return nil
}

// WriteEnd signals the end of the SSE stream
func (ssw *SSEStreamWriter) WriteEnd() error {
	if ssw.closed {
		return nil
	}

	if _, err := fmt.Fprint(ssw.w, "data: [DONE]\n\n"); err != nil {
		return fmt.Errorf("failed to write end marker: %w", err)
	}

	if ssw.flusher != nil {
		ssw.flusher.Flush()
	}

	return nil
}

// Close closes the SSE stream writer
func (ssw *SSEStreamWriter) Close() error {
	ssw.closed = true
	return nil
}
