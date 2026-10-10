package apiclient

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

// Event is one message from the server's event stream, such as
// "files:changed" or "space:changed".
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// FilesChangedPayload is the payload of a "files:changed" event: the ids of
// the buckets that changed, none meaning any bucket.
type FilesChangedPayload struct {
	BucketIds []string `json:"bucket_ids,omitempty"`
}

// ErrEventsRefused is returned by FollowEvents when the server refuses the
// stream: the token is not valid, or was revoked while it was open.
var ErrEventsRefused = errors.New("the server refused the event stream")

const (
	eventsMinDelay = time.Second
	eventsMaxDelay = 30 * time.Second
	// The server sends a keep-alive every 5s; silence for longer than this
	// means the connection is gone.
	eventsIdleTimeout = 20 * time.Second
)

// FollowEvents holds the server's event stream (/api/events) open, calling
// onEvent for each event, until ctx ends. A dropped stream is opened again
// with backoff; onState, when given, is told each time the stream opens or
// closes, so a caller can catch up on what it may have missed. It returns
// ctx's error, or ErrEventsRefused when the server refuses the token.
func (c *ApiClient) FollowEvents(ctx context.Context, onEvent func(Event), onState func(connected bool)) error {
	hc, err := c.rawClient()
	if err != nil {
		return err
	}
	delay := eventsMinDelay
	for {
		opened, err := followOnce(ctx, hc, onEvent, onState)
		if errors.Is(err, ErrEventsRefused) {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if opened {
			delay = eventsMinDelay
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay = min(delay*2, eventsMaxDelay)
	}
}

// followOnce reads one connection of the stream until it ends; opened says
// whether it got as far as being open.
func followOnce(ctx context.Context, hc *rest.HTTPClient, onEvent func(Event), onState func(bool)) (bool, error) {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resp, err := hc.DoRaw(connCtx, http.MethodGet, "/api/events", nil, 0, map[string]string{
		"Accept":        "text/event-stream",
		"Cache-Control": "no-cache",
	})
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return false, ErrEventsRefused
	case resp.StatusCode != http.StatusOK:
		return false, fmt.Errorf("event stream: status %d", resp.StatusCode)
	}

	if onState != nil {
		onState(true)
		defer onState(false)
	}

	// A connection that goes silent is closed, so it is opened again.
	idle := time.AfterFunc(eventsIdleTimeout, cancel)
	defer idle.Stop()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	name := ""
	var data []string
	for sc.Scan() {
		idle.Reset(eventsIdleTimeout)
		line := sc.Text()
		if line == "" {
			// The end of an event.
			if name == "message" || (name == "" && len(data) > 0) {
				var ev Event
				if json.Unmarshal([]byte(strings.Join(data, "\n")), &ev) == nil {
					if ev.Type == "auth:required" {
						return true, ErrEventsRefused
					}
					onEvent(ev)
				}
			}
			name, data = "", data[:0]
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // a comment: the keep-alive
		}
		field, value, _ := strings.Cut(line, ":")
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "event":
			name = value
		case "data":
			data = append(data, value)
		}
	}
	return true, sc.Err()
}
