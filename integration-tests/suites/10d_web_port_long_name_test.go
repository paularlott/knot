//go:build integration

package suites

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
	"github.com/paularlott/knot/internal/util/hostlabel"
)

// TestWebPortLongSpaceName reaches the web port of a space whose name is
// too long for its wildcard host: <user>--<name>--<port> would pass the 63
// characters DNS allows a label, so no browser could resolve it. The host
// the web interface and events give, the name shortened with a hash, reaches
// the space; the full-length host still does where something resolves it;
// a shortened name with the wrong hash reaches nothing.
func TestWebPortLongSpaceName(t *testing.T) {
	harness.Feature(t, "web-ports")
	lan := harness.LanIP()
	s, err := harness.StartServerAt(cfg, bins, "longhost", lan, "--wildcard-domain", "*."+lan)
	if err != nil {
		t.Fatalf("boot server: %v", err)
	}
	t.Cleanup(s.Stop)
	admin, err := harness.ProvisionAdmin(s, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	tmpl, err := harness.CreateTemplate(s, admin.Client, uniqueName("it_long_host"), harness.TemplateOptions{Port: 8080})
	if err != nil {
		t.Fatal(err)
	}

	const name = "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan"
	full := "admin--" + strings.ToLower(name) + "--8080"
	if len(full) <= hostlabel.MaxLen {
		t.Fatalf("the name is too short to test with: %d characters", len(full))
	}
	space := harness.CreateSpace(t, admin.Client, name, tmpl, admin.Id)
	harness.DeleteSpaceAsync(t, admin.Client, space)
	harness.WaitForSpaceReady(t, s, admin.Client, space)

	harness.RunCommand(t, admin.Client, space, 30, "sh", "-c", "mkdir -p /tmp/www && echo long-name-ok > /tmp/www/index.html && cd /tmp/www && (nohup python3 -m http.server 8080 >/dev/null 2>&1 &)")
	if !waitForCond(60, func() bool {
		out, err := harness.TryRunCommand(admin.Client, space, 20, "sh", "-c", "curl -s http://127.0.0.1:8080/ || true")
		return err == nil && strings.Contains(out, "long-name-ok")
	}) {
		t.Fatal("the web server in the space never came up")
	}

	_, port, _ := strings.Cut(strings.TrimPrefix(s.BaseURL, "http://"), ":")
	client := &http.Client{Timeout: 30 * time.Second}
	get := func(label string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("GET", s.BaseURL+"/", nil)
		req.Host = label + "." + lan + ":" + port
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("GET %s: %v", req.Host, err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(body)
	}

	short := hostlabel.Label("admin", name, "8080")
	if len(short) > hostlabel.MaxLen {
		t.Fatalf("the label is still %d characters: %s", len(short), short)
	}
	var code int
	var body string
	if !waitForCond(60, func() bool { code, body = get(short); return code == http.StatusOK }) {
		t.Fatalf("the shortened host %s = %d %q", short, code, body)
	}
	if !strings.Contains(body, "long-name-ok") {
		t.Errorf("the shortened host reached something else: %q", body)
	}
	if code, body := get(full); code != http.StatusOK || !strings.Contains(body, "long-name-ok") {
		t.Errorf("the full-length host = %d %q", code, body)
	}
	wrong := short[:strings.LastIndex(short, "--")-8] + "00000000--8080"
	if code, _ := get(wrong); code != http.StatusNotFound {
		t.Errorf("a shortened name with the wrong hash = %d, want 404", code)
	}
}
