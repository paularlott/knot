package mcp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

// NotifyToolsChanged must reach clients of the public /mcp endpoint, not only
// the internal server InitializeMCPServer returns: script CRUD handlers call it
// so external MCP clients re-fetch their tool lists.
func TestNotifyToolsChangedReachesPublicEndpoint(t *testing.T) {
	prev := config.GetServerConfig()
	// LeafNode skips the permission gate; ApiAuth passes the context user through.
	config.SetServerConfig(&config.ServerConfig{LeafNode: true})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	t.Cleanup(func() { SetServer() })

	routes := &http.ServeMux{}
	InitializeMCPServer(routes, true, nil)

	user := &model.User{Id: "u1", Username: "t"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		routes.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "user", user)))
	}))
	defer srv.Close()

	changed := make(chan struct{}, 8)
	client := mcp.NewClient(srv.URL+"/mcp", nil, "knot")
	client.OnToolsChanged(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	client.EnableNotifications()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}

	// The notification stream opens in the background: notify until it lands.
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		NotifyToolsChanged()
		select {
		case <-changed:
			return
		case <-ctx.Done():
			t.Fatal("no notifications/tools/list_changed reached the /mcp client")
		case <-tick.C:
		}
	}
}
