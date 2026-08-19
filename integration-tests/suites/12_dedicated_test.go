//go:build integration

package suites

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/cli/env"
	"github.com/paularlott/knot/integration-tests/harness"
)

// bootDedicated starts a purpose-configured server and provisions its admin.
func bootDedicated(t *testing.T, name string, extraArgs ...string) (*harness.Server, *harness.User) {
	t.Helper()
	s, err := harness.StartServer(cfg, bins, name, extraArgs...)
	if err != nil {
		t.Fatalf("boot %s server: %v", name, err)
	}
	t.Cleanup(s.Stop)
	adminUser, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision admin on %s: %v", name, err)
	}
	return s, adminUser
}

// fakeOpenAI serves a minimal OpenAI-compatible chat completion endpoint.
func fakeOpenAI(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "data: "+`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"integration-reply"},"finish_reason":null}]}`+"\n\n")
		fmt.Fprint(w, "data: "+`{"id":"1","object":"chat.completion.chunk","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChatOpenAIEndpoint(t *testing.T) {
	harness.Feature(t, "chat")
	fake := fakeOpenAI(t)
	s, adminUser := bootDedicated(t, "chat",
		"--chat-enabled",
		"--chat-openai-endpoints",
		"--chat-provider", "openai",
		"--chat-type", "openai",
		"--chat-base-url", fake.URL,
		"--chat-model", "it-test-model",
		"--chat-api-key", "it-test-key",
	)

	// The server exposes an OpenAI-compatible /v1/chat/completions that
	// proxies to the configured backend.
	body := `{"model":"it-test-model","messages":[{"role":"user","content":"say hi"}],"stream":true}`
	req, _ := http.NewRequest("POST", s.BaseURL+"/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+adminUser.Token)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	_ = env.Load()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("chat completions: %v", err)
	}
	defer resp.Body.Close()
	mustEqual(t, "chat status", resp.StatusCode, 200)

	scanner := bufio.NewScanner(resp.Body)
	sawReply := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") && contains(line, "integration-reply") {
			sawReply = true
		}
	}
	if !sawReply {
		t.Fatal("expected reply content in chat completion stream")
	}
}
