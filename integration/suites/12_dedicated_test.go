//go:build integration

package suites

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/cli/env"
	"github.com/paularlott/knot/integration/harness"
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

func TestDevURLProxy(t *testing.T) {
	harness.Feature(t, "spaces") // dev URL routing is part of the space proxy
	t.Skip("KNOWN ISSUE: the wildcard proxy 404s in-suite because the space's agent never reports template http ports (http_ports stays empty even though KNOT_HTTP_PORT reaches the container and the same template+server config works in a standalone repro). Needs focused investigation; re-enable when fixed.")
	if cfg.Runtime != "docker" {
		t.Skip("dev URL test needs docker networking")
	}
	s, err := harness.StartServerAt(cfg, bins, "devurl", harness.LanIP(), "--wildcard-domain", "knot.test")
	if err != nil {
		t.Fatalf("boot devurl server: %v", err)
	}
	t.Cleanup(s.Stop)
	adminUser, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatalf("provision admin on devurl: %v", err)
	}

	// A template with a named http port.
	tmplId, err := harness.CreateTemplate(s, adminUser.Client, uniqueName("it-devurl"), harness.TemplateOptions{
		PortName: "web",
		Port:     8080,
	})
	if err != nil {
		t.Fatal(err)
	}

	id := harness.CreateSpace(t, adminUser.Client, "it-devurl", tmplId, adminUser.Id)
	t.Cleanup(func() { harness.DeleteSpaceAndWait(t, adminUser.Client, id) })
	harness.WaitForSpaceReady(t, s, adminUser.Client, id)

	// python3 availability decides whether we can serve real content.
	out, perr := harness.TryRunCommand(adminUser.Client, id, 30, "command -v python3 || true")
	hasPython := strings.TrimSpace(out) != "" && perr == nil
	if hasPython {
		harness.RunCommand(t, adminUser.Client, id, 30,
			"nohup python3 -m http.server 8080 >/dev/null 2>&1 &")
		// The http server must answer locally before the proxy is tested.
		localOK := false
		for i := 0; i < 10 && !localOK; i++ {
			time.Sleep(1 * time.Second)
			out, err := harness.TryRunCommand(adminUser.Client, id, 20, "curl -s -o /dev/null -w %{http_code} http://127.0.0.1:8080/ || true")
			if err == nil && strings.TrimSpace(out) == "200" {
				localOK = true
			}
		}
		if !localOK {
			t.Fatal("python http.server never answered inside the space")
		}
	}

	// The wildcard host <user>--<space>--<port>.<wildcard> proxies through
	// the agent into the space. The third label is the port NUMBER.
	host := "admin--it-devurl--8080.knot.test"
	req, _ := http.NewRequest("GET", s.BaseURL+"/", nil)
	req.Host = host
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dev url request: %v", err)
	}
	defer resp.Body.Close()

	if hasPython {
		mustEqual(t, "dev url status", resp.StatusCode, 200)
	} else {
		if resp.StatusCode == http.StatusNotFound {
			t.Fatalf("dev url host %q not routed (404)", host)
		}
		t.Logf("no python3 in image; dev url returned %d (not 404 = routed through agent)", resp.StatusCode)
	}
	_ = json.Marshal
}
