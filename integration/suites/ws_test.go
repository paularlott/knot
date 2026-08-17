//go:build integration

package suites

import (
	"context"
	"net/http"

	"github.com/gorilla/websocket"
)

// wsDial opens a websocket with a bearer token (same as apiclient
// ConnectWebSocket, exposed here for tests).
func wsDial(ctx context.Context, token, url string) (*websocket.Conn, *http.Response, error) {
	header := http.Header{}
	header.Set("Authorization", "Bearer "+token)
	dialer := websocket.Dialer{}
	return dialer.DialContext(ctx, url, header)
}
