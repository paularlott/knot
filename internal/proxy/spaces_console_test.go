package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
)

// TestConsoleReportsFailuresInTheTerminal covers the console's failure UX:
// the websocket is upgraded before the console preconditions run, so a
// failure arrives as a readable [console error] line rather than a popup
// that dies silently. The exact reason depends on the host (this test hits
// the domain-state check, which needs virsh); the assertion is only that a
// reason arrives.
func TestConsoleReportsFailuresInTheTerminal(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })

	db := database.GetInstance()

	user := model.NewUser("console-tester", "tester@example.com", "password", nil, nil, "", "/bin/sh", "", 1, "", 0, 0, 0)
	if err := db.SaveUser(user, nil); err != nil {
		t.Fatal(err)
	}

	template := model.NewTemplate("kvm-console-test", "", "image: base", "", user.Id, nil,
		model.PlatformKvm, true, false, false, false, false, false, "", "",
		0, 0, false, nil, nil, false, true, 0, "disabled", "", nil)
	if err := db.SaveTemplate(template, nil); err !=nil {
		t.Fatal(err)
	}

	space := model.NewSpace("console-space", "", user.Id, template.Id, "/bin/sh", &[]model.AltNameEntry{}, "", "", nil)
	space.ContainerId = "user-console-space"
	if err := db.SaveSpace(space, []string{}); err != nil {
		t.Fatal(err)
	}

	// Route through a mux (the handler reads the space id via PathValue) and
	// inject the authenticated user into the context as ApiAuth would.
	router := http.NewServeMux()
	router.HandleFunc("GET /proxy/spaces/{space_id}/console", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(context.WithValue(r.Context(), "user", user))
		HandleSpacesConsoleProxy(w, r)
	})
	server := httptest.NewServer(router)
	defer server.Close()

	url := strings.Replace(server.URL, "http://", "ws://", 1) + "/proxy/spaces/" + space.Id + "/console"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("failed to open console websocket: %v", err)
	}
	defer conn.Close()

	conn.SetReadLimit(4096)
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("expected an error message before the close, got: %v", err)
	}
	if !strings.Contains(string(message), "[console error]") {
		t.Fatalf("expected a [console error] line, got: %q", string(message))
	}
}
