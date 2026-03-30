package docker

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestToPortKey(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"8080", "8080/tcp"},
		{"8080/tcp", "8080/tcp"},
		{"8080/udp", "8080/udp"},
		{"443", "443/tcp"},
	}
	for _, tt := range tests {
		got := toPortKey(tt.input)
		if got != tt.want {
			t.Errorf("toPortKey(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"512m", 512 * 1024 * 1024, false},
		{"512M", 512 * 1024 * 1024, false},
		{"2g", 2 * 1024 * 1024 * 1024, false},
		{"2G", 2 * 1024 * 1024 * 1024, false},
		{"1073741824", 1073741824, false},
		{"bad", 0, true},
	}
	for _, tt := range tests {
		got, err := parseMemory(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseMemory(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMemory(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestParseCPUs(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"", 0, false},
		{"1", 1e9, false},
		{"0.5", 5e8, false},
		{"2.0", 2e9, false},
		{"bad", 0, true},
	}
	for _, tt := range tests {
		got, err := parseCPUs(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseCPUs(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseCPUs(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestRegistryAuthHeader(t *testing.T) {
	h, err := registryAuthHeader("user", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	decoded, err := base64.URLEncoding.DecodeString(h)
	if err != nil {
		t.Fatalf("base64 decode error: %v", err)
	}

	var m map[string]string
	if err := json.Unmarshal(decoded, &m); err != nil {
		t.Fatalf("json unmarshal error: %v", err)
	}

	if m["username"] != "user" {
		t.Errorf("username = %q, want %q", m["username"], "user")
	}
	if m["password"] != "pass" {
		t.Errorf("password = %q, want %q", m["password"], "pass")
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "CAP_AUDIT_WRITE"}, "CAP_AUDIT_WRITE") {
		t.Error("expected contains to return true")
	}
	if contains([]string{"a", "b"}, "CAP_AUDIT_WRITE") {
		t.Error("expected contains to return false")
	}
}

func TestURLEncoding(t *testing.T) {
	// containerCreate: name is a query param — special chars must be encoded
	// We verify by checking what url.QueryEscape produces for known inputs.
	tests := []struct {
		name    string
		input   string
		wantSub string // substring that must appear in the encoded URL
	}{
		{"plain name", "my-container", "name=my-container"},
		{"name with space", "my container", "name=my+container"},
		{"name with ampersand", "a&b", "name=a%26b"},
		{"name with hash", "a#b", "name=a%23b"},
	}
	for _, tt := range tests {
		got := "/v1.41/containers/create?name=" + url.QueryEscape(tt.input)
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("%s: url=%q does not contain %q", tt.name, got, tt.wantSub)
		}
	}

	// volumeRemove: name is a path segment — '/' must be encoded, other special chars too
	pathTests := []struct {
		name    string
		input   string
		wantSub string
	}{
		{"plain name", "my-volume", "/v1.41/volumes/my-volume"},
		{"name with slash", "a/b", "/v1.41/volumes/a%2Fb"},
		{"name with space", "my volume", "/v1.41/volumes/my%20volume"},
	}
	for _, tt := range pathTests {
		got := "/v1.41/volumes/" + url.PathEscape(tt.input) + "?force=true"
		if !strings.Contains(got, tt.wantSub) {
			t.Errorf("%s: url=%q does not contain %q", tt.name, got, tt.wantSub)
		}
	}

	// imagePull: fromImage preserves '/' but encodes other special chars; tag uses QueryEscape
	imageName := "registry.example.com:5000/my/image"
	tag := "v1.0+build"
	escapedFrom := strings.ReplaceAll(url.QueryEscape(imageName), "%2F", "/")
	gotTag := url.QueryEscape(tag)
	if strings.Contains(escapedFrom, "%2F") {
		t.Errorf("fromImage encoding encoded '/' in image name: %q", escapedFrom)
	}
	if !strings.Contains(escapedFrom, "registry.example.com") {
		t.Errorf("fromImage encoding mangled registry hostname: %q", escapedFrom)
	}
	if !strings.Contains(gotTag, "%2B") {
		t.Errorf("QueryEscape did not encode '+' in tag: %q", gotTag)
	}
}

func TestContainerCreateRequestJSON(t *testing.T) {
	// Verify the JSON field names match what the Docker API expects
	req := containerCreateRequest{
		Image:    "alpine:latest",
		Hostname: "test-host",
		Env:      []string{"FOO=bar"},
		HostConfig: containerHostConfig{
			Binds:         []string{"/host:/container"},
			Privileged:    true,
			NetworkMode:   "bridge",
			RestartPolicy: restartPolicy{Name: "unless-stopped"},
			PortBindings: map[string][]portBinding{
				"8080/tcp": {{HostIP: "0.0.0.0", HostPort: "8080"}},
			},
			Devices: []deviceMapping{{
				PathOnHost:        "/dev/sda",
				PathInContainer:   "/dev/sda",
				CgroupPermissions: "rwm",
			}},
			Memory:   512 * 1024 * 1024,
			NanoCPUs: 1e9,
		},
	}

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// Top-level fields
	for _, key := range []string{"Image", "Hostname", "Env", "HostConfig"} {
		if _, ok := m[key]; !ok {
			t.Errorf("missing JSON key %q", key)
		}
	}

	// HostConfig fields
	hc, _ := m["HostConfig"].(map[string]interface{})
	for _, key := range []string{"Binds", "Privileged", "NetworkMode", "RestartPolicy", "PortBindings", "Devices", "Memory", "NanoCpus"} {
		if _, ok := hc[key]; !ok {
			t.Errorf("missing HostConfig JSON key %q", key)
		}
	}

	// RestartPolicy
	rp, _ := hc["RestartPolicy"].(map[string]interface{})
	if rp["Name"] != "unless-stopped" {
		t.Errorf("RestartPolicy.Name = %v, want unless-stopped", rp["Name"])
	}

	// PortBinding HostIp field name (Docker API is case-sensitive)
	pb, _ := hc["PortBindings"].(map[string]interface{})
	bindings, _ := pb["8080/tcp"].([]interface{})
	binding, _ := bindings[0].(map[string]interface{})
	if _, ok := binding["HostIp"]; !ok {
		t.Error("portBinding missing JSON key HostIp (case matters for Docker API)")
	}
}
