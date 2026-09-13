package kvm

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"gopkg.in/yaml.v3"
)

// TestResolveBaseImageAbsolutePaths guards against relative config paths:
// qemu-img resolves a backing file relative to the overlay's directory, so
// every path leaving the backend must be absolute. Reproduces the
// cloud_path="." / images_path="./images" setup.
func TestResolveBaseImageAbsolutePaths(t *testing.T) {
	cloudDir := t.TempDir()
	imagesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cloudDir, "base.qcow2"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	config.SetServerConfig(&config.ServerConfig{
		KVM: config.KVMConfig{
			ImagesPath:     imagesDir,
			CloudImagePath: cloudDir,
		},
	})

	client := NewClient()

	// Bare name resolves to an absolute path in the cloud image dir, .qcow2 appended.
	base, err := client.resolveBaseImage(context.Background(), "base")
	if err != nil {
		t.Fatalf("resolve bare name: %v", err)
	}
	if want := filepath.Join(cloudDir, "base.qcow2"); base != want {
		t.Fatalf("bare name resolved to %q, want %q", base, want)
	}
	if !filepath.IsAbs(base) {
		t.Fatalf("resolved base must be absolute, got %q", base)
	}

	// An exact-name match wins over the .qcow2 candidate.
	if err := os.WriteFile(filepath.Join(cloudDir, "exact.img"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}
	base, err = client.resolveBaseImage(context.Background(), "exact.img")
	if err != nil {
		t.Fatalf("resolve exact name: %v", err)
	}
	if want := filepath.Join(cloudDir, "exact.img"); base != want {
		t.Fatalf("exact name resolved to %q, want %q", base, want)
	}
}

// TestImagesDirAbsolute ensures the working directories are absolutized even
// when configured relative to the server's working directory.
func TestImagesDirAbsolute(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{
		KVM: config.KVMConfig{ImagesPath: "./images", CloudImagePath: "."},
	})

	if !filepath.IsAbs(imagesDir()) {
		t.Fatalf("imagesDir must be absolute, got %q", imagesDir())
	}
	if !filepath.IsAbs(cloudImagesDir()) {
		t.Fatalf("cloudImagesDir must be absolute, got %q", cloudImagesDir())
	}
}

// Regression guard: the installer script must survive YAML marshalling and
// a cloud-init style parse byte-for-byte — a mangled first line would break
// its execution in ways that look nothing like a script bug.
func TestCloudInitUserDataRoundTrip(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{
		URL:           "https://knot.example.com",
		AgentEndpoint: "knot.example.com:17001",
	})

	data := buildCloudInit(
		&model.Template{Platform: model.PlatformKvm},
		&model.Space{Id: "s", Name: "vm", IPAddress: "192.168.50.10"},
		&model.User{Username: "u"},
		&jobSpec{Image: "base", Environment: []string{"TEST=Testing env 1"}},
		&networkPlan{IP: net.ParseIP("192.168.50.10"), PrefixLen: 24,
			Gateway: net.ParseIP("192.168.50.1"),
			DNS:     []net.IP{net.ParseIP("192.168.50.1")}},
	)

	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(data.UserData), &doc); err != nil {
		t.Fatalf("user-data is not valid YAML: %v\n%s", err, data.UserData)
	}

	// The env file must survive being sourced by a real shell — an
	// unquoted value with spaces parses as a command ("TEST=Testing env 1"
	// executes `env 1`), which is exactly how the installer broke.
	var envFile string
	files, _ := doc["write_files"].([]interface{})
	for _, f := range files {
		m, _ := f.(map[string]interface{})
		if m["path"] == "/etc/knot/agent.env" {
			envFile, _ = m["content"].(string)
		}
	}
	if envFile == "" {
		t.Fatalf("agent.env missing from write_files")
	}
	tmp := t.TempDir() + "/agent.env"
	if err := os.WriteFile(tmp, []byte(envFile), 0600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("/bin/sh", "-c", ". "+tmp+" && printf '%s' \"$TEST\"").Output()
	if err != nil {
		t.Fatalf("agent.env is not sh-sourceable: %v\n%s", err, envFile)
	}
	if string(out) != "Testing env 1" {
		t.Fatalf("TEST did not survive sourcing: %q\n%s", out, envFile)
	}

	for _, f := range files {
		m, _ := f.(map[string]interface{})
		if m["path"] == "/usr/local/sbin/knot-agent-install.sh" {
			content, _ := m["content"].(string)
			if content != agentInstallScript {
				t.Fatalf("installer script did not survive YAML round-trip")
			}
			if !strings.HasPrefix(content, "#!/bin/sh\n") {
				t.Fatalf("installer script must start with an intact shebang, got: %q", content[:40])
			}
			return
		}
	}
	t.Fatalf("installer script missing from write_files: %v", doc["write_files"])
}

func TestKvmResolvers(t *testing.T) {
	template := &model.Template{
		Platform:        model.PlatformKvm,
		KvmNetworkCidr:  "192.168.50.0/24",
		KvmIPRangeStart: "192.168.50.10",
		KvmIPRangeEnd:   "192.168.50.100",
		KvmGateway:      "192.168.50.1",
	}
	space := &model.Space{IPAddress: "192.168.50.10"}

	// Default: Cloudflare pair, no gateway injected.
	config.SetServerConfig(&config.ServerConfig{})
	plan, err := resolveNetwork(template, space)
	if err != nil {
		t.Fatalf("default resolvers: %v", err)
	}
	if len(plan.DNS) != 2 || plan.DNS[0].String() != "1.1.1.1" || plan.DNS[1].String() != "1.0.0.1" {
		t.Fatalf("default resolvers = %v", plan.DNS)
	}

	// Override: admin list wins verbatim.
	config.SetServerConfig(&config.ServerConfig{
		KVM: config.KVMConfig{Resolvers: []string{"192.168.8.1", " 9.9.9.9 "}},
	})
	plan, err = resolveNetwork(template, space)
	if err != nil {
		t.Fatalf("override resolvers: %v", err)
	}
	if len(plan.DNS) != 2 || plan.DNS[0].String() != "192.168.8.1" || plan.DNS[1].String() != "9.9.9.9" {
		t.Fatalf("override resolvers = %v", plan.DNS)
	}

	// Invalid entries are a hard error naming the entry.
	config.SetServerConfig(&config.ServerConfig{
		KVM: config.KVMConfig{Resolvers: []string{"not-an-ip"}},
	})
	if _, err := resolveNetwork(template, space); err == nil || !strings.Contains(err.Error(), "not-an-ip") {
		t.Fatalf("expected resolver error naming the entry, got %v", err)
	}
}

func TestAttachmentArg(t *testing.T) {
	if got := (&attachment{Mode: "bridge", Name: "br0"}).arg(); got != "bridge=br0,model=virtio" {
		t.Fatalf("bridge arg = %q", got)
	}
	if got := (&attachment{Mode: "network", Name: "default"}).arg(); got != "network=default,model=virtio" {
		t.Fatalf("network arg = %q", got)
	}
}

// Regression: NAT mode builds the seed with a nil network plan — a nil
// dereference here panicked (and killed) the server on space start.
func TestBuildCloudInitNATNilPlan(t *testing.T) {
	config.SetServerConfig(&config.ServerConfig{
		URL:           "https://knot.example.com",
		AgentEndpoint: "knot.example.com:17001",
	})

	template := &model.Template{Platform: model.PlatformKvm, KvmNetworkMode: model.KvmNetworkModeNat}
	space := &model.Space{Id: "test-space", Name: "natvm", IPAddress: ""}
	user := &model.User{Username: "paul", ServicePassword: "secret"}

	data := buildCloudInit(template, space, user, &jobSpec{Image: "base"}, nil)
	if data.UserData == "" || !strings.Contains(data.UserData, "#cloud-config") {
		t.Fatalf("NAT seed must still carry user-data: %q", data.UserData)
	}
	if data.NetworkConfig != "" {
		t.Fatalf("NAT seed must not carry network-config: %q", data.NetworkConfig)
	}
	if !strings.Contains(data.UserData, "package_upgrade: false") || !strings.Contains(data.UserData, "package_update: false") {
		t.Fatalf("user-data must pin package update/upgrade behaviour: %q", data.UserData)
	}
	if !strings.Contains(data.UserData, "ExecStartPre=/bin/sh /usr/local/sbin/knot-agent-install.sh") {
		t.Fatalf("installer ExecStartPre missing: %q", data.UserData)
	}
	if !strings.Contains(data.UserData, "ExecStart=/bin/sh /usr/local/sbin/knot-agent-run.sh") {
		t.Fatalf("unit must launch via the run script: %q", data.UserData)
	}
	if !strings.Contains(data.UserData, "sudo -u paul -H") {
		t.Fatalf("run script must drop to the owner via sudo: %q", data.UserData)
	}
	if strings.Contains(data.UserData, "WorkingDirectory=") {
		t.Fatalf("unit must not chdir (the sudo launcher handles the user): %q", data.UserData)
	}
	if strings.Contains(data.UserData, "packages:") {
		t.Fatalf("NAT must not install packages: %q", data.UserData)
	}
	if !strings.Contains(data.UserData, "/run/knot-agent-fetched") {
		t.Fatalf("installer must gate re-fetching to once per boot: %q", data.UserData)
	}
	if !strings.Contains(data.UserData, "install -d -o paul -g paul -m 0750 /home/paul") {
		t.Fatalf("runcmd must normalize the owner's home before the unit starts: %q", data.UserData)
	}
	if !strings.Contains(data.MetaData, "instance-id: knot-test-space-") {
		t.Fatalf("meta-data must carry the hashed instance-id: %q", data.MetaData)
	}
}
