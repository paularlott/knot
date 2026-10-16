package proxy

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// A space's web server is the user's own code, so it must never receive
// knot's session cookie: with it, it could act as whoever opened the page.
func TestAgentReverseProxyStripsKnotCookies(t *testing.T) {
	type seen struct{ cookie, auth string }
	got := make(chan seen, 1)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- seen{cookie: r.Header.Get("Cookie"), auth: r.Header.Get("Authorization")}
		if r.Header.Get("Upgrade") == "websocket" {
			w.Header().Set("Upgrade", "websocket")
			w.Header().Set("Connection", "Upgrade")
			w.WriteHeader(http.StatusSwitchingProtocols)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	for _, tt := range []struct {
		name    string
		upgrade bool
	}{{"request", false}, {"websocket upgrade", true}} {
		t.Run(tt.name, func(t *testing.T) {
			// The proxy sends everything down the one stream, as it does to
			// an agent.
			stream, err := net.Dial("tcp", strings.TrimPrefix(backend.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			target, _ := url.Parse("http://127.0.0.1/")
			token := "Basic dmVjOnNlY3JldA=="
			proxy := CreateAgentReverseProxy(target, stream, &token, "admin--space--vnc.knot.example.com")

			front := httptest.NewServer(proxy)
			defer front.Close()

			conn, err := net.Dial("tcp", strings.TrimPrefix(front.URL, "http://"))
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			req, _ := http.NewRequest("GET", front.URL+"/app", nil)
			req.Header.Add("Cookie", "app=1; __KNOT_WEBUI_SESSION=secret-session")
			req.Header.Add("Cookie", "__KNOT_OAUTH_PKCE=p; other=2")
			if tt.upgrade {
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "websocket")
				req.Header.Set("Sec-WebSocket-Version", "13")
				req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			}
			if err := req.Write(conn); err != nil {
				t.Fatal(err)
			}
			resp, err := http.ReadResponse(bufio.NewReader(conn), req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()

			s := <-got
			if strings.Contains(s.cookie, "__KNOT_") || strings.Contains(s.cookie, "secret-session") {
				t.Errorf("space received knot cookies: %q", s.cookie)
			}
			if s.cookie != "app=1; other=2" {
				t.Errorf("space's own cookies = %q, want %q", s.cookie, "app=1; other=2")
			}
			if s.auth != token {
				t.Errorf("Authorization = %q, want the proxy's token", s.auth)
			}
		})
	}
}
