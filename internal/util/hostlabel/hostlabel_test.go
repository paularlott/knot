package hostlabel

import (
	"strings"
	"testing"
)

func TestLabel(t *testing.T) {
	// A label that fits is the names as they are, lowercased.
	if got := Label("Alice", "Web-App", "8080"); got != "alice--web-app--8080" {
		t.Errorf("short label: %q", got)
	}
	exact := strings.Repeat("a", MaxLen-len("bob----80"))
	if got := Label("bob", exact, "80"); got != "bob--"+exact+"--80" || len(got) != MaxLen {
		t.Errorf("a label of exactly %d characters was changed: %q", MaxLen, got)
	}

	long := "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan"
	for _, c := range []struct{ user, port string }{{"bob", "80"}, {"bob", "65535"}, {"bob", "vnc"}, {strings.Repeat("u", 30), "8080"}} {
		got := Label(c.user, long, c.port)
		if len(got) > MaxLen {
			t.Errorf("%s/%s: %d characters: %q", c.user, c.port, len(got), got)
		}
		parts := strings.Split(got, "--")
		if len(parts) != 3 || parts[0] != c.user || parts[2] != c.port {
			t.Errorf("%s/%s: the label does not split into user, name and port: %q", c.user, c.port, got)
			continue
		}
		if !Matches(parts[1], long) {
			t.Errorf("%s/%s: %q does not match the name it was made from", c.user, c.port, parts[1])
		}
		if !strings.HasPrefix(strings.ToLower(long), strings.TrimSuffix(parts[1], "-"+Hash(long))) {
			t.Errorf("%s/%s: %q does not start with the name", c.user, c.port, parts[1])
		}
	}
}

// A prefix cut at a hyphen loses the hyphen, so the label holds "--" only
// between its parts.
func TestLabelNoDoubleHyphen(t *testing.T) {
	user, port := "bob", "80"
	// The cut falls just after the hyphen in "x-".
	keep := MaxLen - len(user) - len(port) - 4 - hashLen - 1
	name := strings.Repeat("x", keep-1) + "-" + strings.Repeat("y", 40)
	got := Label(user, name, port)
	if strings.Count(got, "--") != 2 {
		t.Errorf("the label holds another --: %q", got)
	}
	if !Matches(strings.Split(got, "--")[1], name) {
		t.Errorf("%q does not match its name", got)
	}
}

func TestMatches(t *testing.T) {
	name := "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan"
	other := "Ticket123456-review-the-quarterly-report-and-sign-off-the-plan2"
	part := strings.Split(Label("bob", name, "80"), "--")[1]
	if !Matches(part, name) || !Matches(strings.ToUpper(part), name) {
		t.Errorf("%q should match %q", part, name)
	}
	if Matches(part, other) {
		t.Errorf("%q matched another name with the same start", part)
	}
	for _, p := range []string{"web-app", "web-app-1234567", "web-app-1234567g", "-12345678", ""} {
		if Matches(p, name) || (p != "" && p != "-12345678" && Shortened(p)) {
			t.Errorf("%q taken for a shortened name", p)
		}
	}
	// Pinned: the web interface and the MCP tools make the same.
	for in, want := range map[string]string{"a": "e40c292c", "web-app": "ccb053a7", "WEB-APP": "ccb053a7", name: "35171436"} {
		if h := Hash(in); h != want {
			t.Errorf("Hash(%q) = %s, want %s", in, h, want)
		}
	}
	if got := Label("bob", name, "8080"); got != "bob--ticket123456-review-the-quarterly-report-an-35171436--8080" {
		t.Errorf("pinned label %q", got)
	}
}

func TestCheckTunnel(t *testing.T) {
	if err := CheckTunnel("Alice", "demo"); err != nil {
		t.Errorf("a short tunnel refused: %v", err)
	}
	user := strings.Repeat("u", 24)
	fits := strings.Repeat("n", MaxLen-len(user)-2)
	if err := CheckTunnel(user, fits); err != nil {
		t.Errorf("a tunnel of exactly %d characters refused: %v", MaxLen, err)
	}
	err := CheckTunnel(user, fits+"x")
	if err == nil || !strings.Contains(err.Error(), "at most 37 characters") {
		t.Errorf("a tunnel one character too long: %v", err)
	}
}
