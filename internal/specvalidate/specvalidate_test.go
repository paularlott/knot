package specvalidate

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/database/model"
)

// containsIssue reports whether issues contains an entry with the given line and
// a message containing all of the given substrings.
func containsIssue(issues []Issue, line int, substrs ...string) bool {
	for _, is := range issues {
		if line != 0 && is.Line != line {
			continue
		}
		ok := true
		for _, s := range substrs {
			if !strings.Contains(is.Message, s) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

func TestValidatePortMapping(t *testing.T) {
	cases := []struct {
		name    string
		port    string
		wantErr bool
		substr  string
	}{
		{"valid tcp", "8080:80/tcp", false, ""},
		{"valid no protocol", "8080:80", false, ""},
		{"missing colon", "8080", true, "format hostPort:containerPort"},
		{"too many colons", "1:2:3", true, "format hostPort:containerPort"},
		{"non-numeric host", "abc:80", true, "invalid host port"},
		{"out of range", "70000:80", true, "invalid host port"},
		{"bad protocol", "80:80/icmp", true, "tcp or udp"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validatePortMapping(c.port)
			if c.wantErr && err == nil {
				t.Fatalf("expected error for %q, got nil", c.port)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got %v", c.port, err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.substr) {
				t.Fatalf("error %q should contain %q", err.Error(), c.substr)
			}
		})
	}
}

func TestValidateCapability(t *testing.T) {
	cases := []struct {
		name    string
		value   string
		wantErr bool
		substr  string
	}{
		{"valid", "CAP_NET_ADMIN", false, ""},
		{"valid numeric", "CAP_SYS_ADMIN", false, ""},
		{"missing CAP prefix", "NET_ADMIN", true, "must start with CAP_"},
		{"lowercase rejected", "CAP_net_admin", true, "uppercase"},
		{"empty", "   ", true, "empty"},
		{"just CAP_", "CAP_", true, "missing the name"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateCapability(c.value)
			if c.wantErr && err == nil {
				t.Fatalf("expected error for %q", c.value)
			}
			if !c.wantErr && err != nil {
				t.Fatalf("expected no error for %q, got %v", c.value, err)
			}
			if c.wantErr && !strings.Contains(err.Error(), c.substr) {
				t.Fatalf("error %q should contain %q", err.Error(), c.substr)
			}
		})
	}
}

func TestValidateEnvEntry(t *testing.T) {
	if err := validateEnvEntry("FOO=bar"); err != nil {
		t.Fatalf("valid env rejected: %v", err)
	}
	if err := validateEnvEntry("=bar"); err == nil {
		t.Fatal("expected error for missing key")
	}
	if err := validateEnvEntry("NOEQUALS"); err == nil {
		t.Fatal("expected error for entry without =")
	}
}

func TestValidateHostContainerPair(t *testing.T) {
	if err := validateHostContainerPair("/host:/container", "volume"); err != nil {
		t.Fatalf("valid pair rejected: %v", err)
	}
	if err := validateHostContainerPair("onlyhost", "device"); err == nil {
		t.Fatal("expected error for pair missing colon")
	}
	if err := validateHostContainerPair(":/container", "volume"); err == nil {
		t.Fatal("expected error for empty host path")
	}
}

func TestValidateAddHostEntry(t *testing.T) {
	if err := validateAddHostEntry("myhost:1.2.3.4"); err != nil {
		t.Fatalf("valid add_host rejected: %v", err)
	}
	if err := validateAddHostEntry("myhost:notanip"); err == nil {
		t.Fatal("expected error for invalid IP")
	}
}

func TestLineFromError(t *testing.T) {
	cases := []struct{ in string; want int }{
		{"yaml: line 5: could not find expected ':'", 5},
		{"yaml: unmarshal errors:\n  line 3: cannot unmarshal", 3},
		{"no line here", 0},
		{"", 0},
	}
	for _, c := range cases {
		if got := lineFromError(errStr(c.in)); got != c.want {
			t.Fatalf("lineFromError(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

type errStr string

func (e errStr) Error() string { return string(e) }

// TestValidateLocalContainerJob_LineCapture verifies that issues for list items
// carry the line of the offending item, so the editor can annotate accurately.
func TestValidateLocalContainerJob_LineCapture(t *testing.T) {
	// Line numbers below are 1-based and assume the leading newline.
	issues := validateLocalContainerJob(`
image: nginx
ports:
  - "abc:80"
environment:
  - "BAD"
cap_add:
  - NET_ADMIN
`)
	if !containsIssue(issues, 4, "port", "abc:80") {
		t.Fatalf("expected port issue on line 4, got %+v", issues)
	}
	if !containsIssue(issues, 6, "environment", "BAD") {
		t.Fatalf("expected environment issue on line 6, got %+v", issues)
	}
	if !containsIssue(issues, 8, "NET_ADMIN", "CAP_") {
		t.Fatalf("expected cap_add issue on line 8, got %+v", issues)
	}
}

func TestValidateLocalContainerJob_UnknownFieldAndImage(t *testing.T) {
	issues := validateLocalContainerJob(`ports:
  - "80:80"
`)
	// image absent -> "image must be set"
	if !containsIssue(issues, 0, "image must be set") {
		t.Fatalf("expected image-required issue, got %+v", issues)
	}

	issues = validateLocalContainerJob(`image: nginx
bogus_field: 1
`)
	if !containsIssue(issues, 2, "unknown field", "bogus_field") {
		t.Fatalf("expected unknown-field issue on line 2, got %+v", issues)
	}
}

func TestValidateLocalContainerJob_StructuralErrors(t *testing.T) {
	// Malformed YAML -> a line-numbered parse issue.
	issues := validateLocalContainerJob(`image: nginx
  bad: : indent
`)
	if len(issues) != 1 || issues[0].Line == 0 {
		t.Fatalf("expected one line-numbered parse issue, got %+v", issues)
	}

	// Non-mapping root.
	issues = validateLocalContainerJob(`just a string`)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "mapping") {
		t.Fatalf("expected mapping issue, got %+v", issues)
	}

	// Empty.
	issues = validateLocalContainerJob(`   `)
	if len(issues) != 1 || !strings.Contains(issues[0].Message, "required") {
		t.Fatalf("expected required issue, got %+v", issues)
	}
}

func TestValidateLocalContainerJob_MalformedList(t *testing.T) {
	issues := validateLocalContainerJob(`image: nginx
ports: not-a-list
`)
	if !containsIssue(issues, 2, "ports must be a list") {
		t.Fatalf("expected 'ports must be a list' on line 2, got %+v", issues)
	}
}

func TestValidateLocalContainerJob_ValidSpec(t *testing.T) {
	issues := ValidateTemplateSpec(model.PlatformContainer, `
image: registry-1.docker.io/library/nginx:latest
ports:
  - "8080:80/tcp"
environment:
  - FOO=bar
volumes:
  - /tmp/cache:/cache
memory: 512M
cpus: "1.5"
cap_add:
  - CAP_AUDIT_WRITE
`, `
volumes:
  app-cache:
`)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}

func TestValidateTemplateSpecNomadUsesParser(t *testing.T) {
	parseCalls := 0
	issues := ValidateTemplateSpecWithNomadParser(
		model.PlatformNomad,
		`job "example" {}`,
		"",
		func(job string) error {
			parseCalls++
			if !strings.Contains(job, `job "example"`) {
				t.Fatalf("unexpected job: %q", job)
			}
			return errStr("nomad parse failed")
		},
	)
	if parseCalls != 1 {
		t.Fatalf("expected parser called once, got %d", parseCalls)
	}
	if len(issues) != 1 || issues[0].Field != "job" {
		t.Fatalf("expected one job issue, got %+v", issues)
	}
}

func TestValidateNomadVolumes_PluginIDAndType(t *testing.T) {
	issues := validateNomadVolumes("definition", `
volumes:
  - name: data
    type: csi
`, true)
	if !containsIssue(issues, 0, "plugin_id") {
		t.Fatalf("expected plugin_id issue, got %+v", issues)
	}

	issues = validateNomadVolumes("definition", `
volumes:
  - name: data
    type: magic
    plugin_id: hostpath
`, true)
	if !containsIssue(issues, 0, "unsupported type", "magic") {
		t.Fatalf("expected unsupported-type issue, got %+v", issues)
	}
}

func TestValidateVolumeSpecLocalRequiresSingleVolume(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformContainer, `
volumes:
  one:
  two:
`)
	if len(issues) != 1 {
		t.Fatalf("expected one issue, got %+v", issues)
	}
}

func TestValidateVolumeSpecLocalRejectsPaths(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformContainer, `
paths:
  - workspace
`)
	if len(issues) == 0 {
		t.Fatal("expected paths to be rejected for standalone volume definitions")
	}
}

func TestValidateVolumeSpecNomadRequiresPluginID(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformNomad, `
volumes:
  - name: data
    type: csi
`)
	if len(issues) == 0 {
		t.Fatal("expected at least one issue")
	}
}

func TestValidateVolumeSpecAppleWithSize(t *testing.T) {
	issues := ValidateVolumeSpec(model.PlatformApple, `
volumes:
  workspace:
    size: 20G
`)
	if len(issues) != 0 {
		t.Fatalf("expected no issues, got %+v", issues)
	}
}
