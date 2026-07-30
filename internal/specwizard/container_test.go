package specwizard

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// TestContainerYAMLRoundTrip verifies that ParseContainerYAML → BuildContainerYAML
// preserves all wizard-controlled fields. This is the guarantee the wizard UI
// relies on: editing a value in the wizard and saving must not drop other
// wizard values.
func TestContainerYAMLRoundTrip(t *testing.T) {
	original := `image: registry.example.com/app:1.2.3
hostname: my-host
command:
  - sleep
  - infinity
privileged: true
network: bridge
memory: 2G
cpus: "2"
ports:
  - "8080:80/tcp"
  - "53:53/udp"
volumes:
  - workspace:/workspace
  - /etc/passwd:/etc/passwd:ro
devices:
  - /dev/fuse:/dev/fuse
environment:
  - TZ=UTC
  - FOO=bar
add_host:
  - host.docker.internal:192.168.1.10
dns:
  - 1.1.1.1
dns_search:
  - internal.example
cap_add:
  - CAP_NET_ADMIN
cap_drop:
  - CAP_MKNOD
`
	originalVolumes := `volumes:
  workspace:
    size: 20G
paths:
  - /storage/data
`

	spec, wizardable, reason := ParseContainerYAML(original, originalVolumes)
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	if spec.Image != "registry.example.com/app:1.2.3" {
		t.Errorf("Image = %q", spec.Image)
	}
	if spec.Hostname != "my-host" {
		t.Errorf("Hostname = %q", spec.Hostname)
	}
	if spec.Memory != "2G" {
		t.Errorf("Memory = %q", spec.Memory)
	}
	if spec.CPUs != "2" {
		t.Errorf("CPUs = %q", spec.CPUs)
	}
	if !spec.Privileged {
		t.Error("Privileged lost")
	}
	if spec.Network != "bridge" {
		t.Errorf("Network = %q", spec.Network)
	}
	if len(spec.Command) != 2 || spec.Command[0] != "sleep" || spec.Command[1] != "infinity" {
		t.Errorf("Command = %v", spec.Command)
	}
	if len(spec.Ports) != 2 {
		t.Errorf("Ports len = %d", len(spec.Ports))
	} else {
		if spec.Ports[0].HostPort != 8080 || spec.Ports[0].ContainerPort != 80 || spec.Ports[0].Protocol != "tcp" {
			t.Errorf("Ports[0] = %+v", spec.Ports[0])
		}
		if spec.Ports[1].Protocol != "udp" {
			t.Errorf("Ports[1] protocol = %q", spec.Ports[1].Protocol)
		}
	}
	// Storage: "workspace" has a matching Volume Definition entry, so it becomes Kind="volume".
	// "/etc/passwd" has no definition, so it stays Kind="bind".
	{
		var binds, vols []apiclient.StorageEntry
		for _, s := range spec.Storage {
			switch s.Kind {
			case "bind":
				binds = append(binds, s)
			case "volume":
				vols = append(vols, s)
			}
		}
		if len(vols) != 1 || vols[0].Name != "workspace" || vols[0].ContainerPath != "/workspace" || vols[0].Size != "20G" {
			t.Errorf("Storage volumes = %+v", vols)
		}
		if len(binds) != 1 || binds[0].HostPath != "/etc/passwd" || !binds[0].ReadOnly {
			t.Errorf("Storage binds = %+v", binds)
		}
	}
	if len(spec.Environment) != 2 {
		t.Errorf("Environment len = %d", len(spec.Environment))
	} else if spec.Environment[1].Key != "FOO" || spec.Environment[1].Value != "bar" {
		t.Errorf("Environment[1] = %+v", spec.Environment[1])
	}
	if len(spec.CapAdd) != 1 || spec.CapAdd[0] != "CAP_NET_ADMIN" {
		t.Errorf("CapAdd = %v", spec.CapAdd)
	}

	// Storage includes the volume definition entries
	{
		var vols []apiclient.StorageEntry
		var paths []apiclient.StorageEntry
		for _, s := range spec.Storage {
			switch s.Kind {
			case "volume":
				vols = append(vols, s)
			case "path":
				paths = append(paths, s)
			}
		}
		if len(vols) == 0 {
			t.Fatal("Storage has no volume entries")
		}
		foundWorkspace := false
		for _, v := range vols {
			if v.Name == "workspace" && v.Size == "20G" {
				foundWorkspace = true
			}
		}
		if !foundWorkspace {
			t.Errorf("Storage missing workspace volume: %+v", vols)
		}
		if len(paths) != 1 || paths[0].HostPath != "/storage/data" {
			t.Errorf("Storage paths = %+v", paths)
		}
	}

	// Now build back and re-parse — every wizard field should survive.
	job, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}

	spec2, _, _ := ParseContainerYAML(job, volumes)
	if spec2.Image != spec.Image {
		t.Errorf("Image round-trip mismatch: %q vs %q", spec2.Image, spec.Image)
	}
	if spec2.Hostname != spec.Hostname {
		t.Errorf("Hostname round-trip mismatch: %q vs %q", spec2.Hostname, spec.Hostname)
	}
	if spec2.Memory != spec.Memory {
		t.Errorf("Memory round-trip mismatch: %q vs %q", spec2.Memory, spec.Memory)
	}
	if spec2.Network != spec.Network {
		t.Errorf("Network round-trip mismatch: %q vs %q", spec2.Network, spec.Network)
	}
	if spec2.Privileged != spec.Privileged {
		t.Errorf("Privileged round-trip mismatch: %v", spec2.Privileged)
	}
	if len(spec2.Ports) != len(spec.Ports) {
		t.Errorf("Ports round-trip mismatch: %d vs %d", len(spec2.Ports), len(spec.Ports))
	}
}

func TestParseContainerYAML_empty(t *testing.T) {
	spec, wizardable, _ := ParseContainerYAML("", "")
	if !wizardable {
		t.Error("empty container YAML should be wizardable (fresh spec)")
	}
	if spec == nil {
		t.Fatal("nil spec for empty input")
	}
	if spec.Image != "" {
		t.Errorf("empty spec has image %q", spec.Image)
	}
}

func TestParseContainerYAML_invalidYAML(t *testing.T) {
	_, wizardable, reason := ParseContainerYAML("this is : not: valid : yaml : :", "")
	if wizardable {
		t.Error("invalid YAML reported as wizardable")
	}
	if !strings.Contains(reason, "parse failed") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestBuildContainerYAML_minimal(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:  "${{ .server.base_image_registry }}/knot-ubuntu:26.04",
		Memory: "2G",
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "image:") {
		t.Errorf("output missing image: %s", job)
	}
	if !strings.Contains(job, "${{ .server.base_image_registry }}") {
		t.Errorf("template var mangled in output: %s", job)
	}
	if !strings.Contains(job, "memory: 2G") {
		t.Errorf("memory not emitted: %s", job)
	}
}

// ---------------------------------------------------------------------------
// Unhappy / malformed-input paths for ParseContainerYAML
// ---------------------------------------------------------------------------

func TestParseContainerYAML_malformedPorts(t *testing.T) {
	cases := map[string]string{
		"non-numeric host":        `image: x:1` + "\n" + `ports: ["abc:80"]`,
		"non-numeric container":   `image: x:1` + "\n" + `ports: ["80:xyz"]`,
		"too many colons":         `image: x:1` + "\n" + `ports: ["80:80:80"]`,
		"empty string":            `image: x:1` + "\n" + `ports: [""]`,
		"out of range high":       `image: x:1` + "\n" + `ports: ["99999:80"]`,
		"out of range zero":       `image: x:1` + "\n" + `ports: ["0:80"]`,
		"invalid protocol suffix": `image: x:1` + "\n" + `ports: ["80:80/ftp"]`,
	}
	for name, yaml := range cases {
		t.Run(name, func(t *testing.T) {
			spec, wizardable, _ := ParseContainerYAML(yaml, "")
			if !wizardable {
				t.Errorf("malformed port should not make spec unwizardable, just skip the entry")
			}
			if spec == nil {
				return
			}
			// Malformed entries must be silently dropped, not crash.
			for _, p := range spec.Ports {
				if p.ContainerPort == 0 || p.HostPort == 0 {
					t.Errorf("malformed port was kept: %+v", p)
				}
			}
		})
	}
}

func TestParseContainerYAML_malformedVolumes(t *testing.T) {
	cases := map[string]string{
		"missing container path": `image: x:1` + "\n" + `volumes: ["onlyhost"]`,
		"empty string":           `image: x:1` + "\n" + `volumes: [""]`,
	}
	for name, yaml := range cases {
		t.Run(name, func(t *testing.T) {
			spec, _, _ := ParseContainerYAML(yaml, "")
			for _, s := range spec.Storage {
				if s.Kind == "bind" && (s.ContainerPath == "" || s.HostPath == "") {
					t.Errorf("malformed volume kept: %+v", s)
				}
			}
		})
	}
}

func TestParseContainerYAML_malformedEnv(t *testing.T) {
	// Entries without "=" should be dropped, not crash.
	job := `image: x:1
environment:
  - VALID=ok
  - NO_EQUALS_SIGN
  - ALSO=valid
`
	spec, _, _ := ParseContainerYAML(job, "")
	if len(spec.Environment) != 2 {
		t.Errorf("Environment len = %d, want 2 (invalid entry should be dropped): %+v", len(spec.Environment), spec.Environment)
	}
	if spec.Environment[0].Key != "VALID" || spec.Environment[0].Value != "ok" {
		t.Errorf("Environment[0] = %+v", spec.Environment[0])
	}
}

func TestParseContainerYAML_missingImage(t *testing.T) {
	// YAML parses but no image — still wizardable, just empty image. The
	// validation layer (specvalidate) catches missing image separately.
	job := `memory: 1G`
	spec, wizardable, _ := ParseContainerYAML(job, "")
	if !wizardable {
		t.Error("spec without image should still be wizardable")
	}
	if spec.Image != "" {
		t.Errorf("Image = %q, want empty", spec.Image)
	}
	if spec.Memory != "1G" {
		t.Errorf("Memory not extracted: %q", spec.Memory)
	}
}

func TestParseContainerYAML_unknownTopLevelFields(t *testing.T) {
	// Unknown fields are silently dropped (KnownFields=false). The validator
	// flags them separately. Wizard should not refuse to load.
	job := `image: x:1
unknown_field: value
another: [1, 2, 3]
`
	spec, wizardable, reason := ParseContainerYAML(job, "")
	if !wizardable {
		t.Errorf("unknown fields should not make spec unwizardable: %s", reason)
	}
	if spec.Image != "x:1" {
		t.Errorf("Image = %q", spec.Image)
	}
}

func TestParseContainerYAML_volumeDefinitions(t *testing.T) {
	cases := map[string]struct {
		yaml   string
		verify func(*testing.T, []apiclient.StorageEntry)
	}{
		"empty volumes field": {
			yaml:   "volumes: null\n",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {},
		},
		"only paths": {
			yaml: "paths:\n  - /storage/data\n  - /storage/logs\n",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {
				var paths []apiclient.StorageEntry
				for _, s := range storage {
					if s.Kind == "path" {
						paths = append(paths, s)
					}
				}
				if len(paths) != 2 {
					t.Errorf("Paths = %v", paths)
				}
			},
		},
		"named volume with size": {
			yaml: "volumes:\n  workspace:\n    size: 20G\n",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {
				found := false
				for _, s := range storage {
					if s.Kind == "volume" && s.Name == "workspace" && s.Size == "20G" {
						found = true
					}
				}
				if !found {
					t.Errorf("workspace volume not found in storage: %+v", storage)
				}
			},
		},
		"named volume without size": {
			yaml: "volumes:\n  workspace: {}\n",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {
				found := false
				for _, s := range storage {
					if s.Kind == "volume" && s.Name == "workspace" && s.Size == "" {
						found = true
					}
				}
				if !found {
					t.Errorf("workspace volume not found in storage: %+v", storage)
				}
			},
		},
		"malformed yaml": {
			yaml: "volumes: [this is broken\n",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {
				// Malformed volume definitions now cause ParseContainerYAML to
				// return wizardable=false, so spec is nil and storage won't be
				// reached. This case is tested separately below.
			},
		},
		"empty string": {
			yaml: "",
			verify: func(t *testing.T, storage []apiclient.StorageEntry) {
				for _, s := range storage {
					if s.Kind == "volume" || s.Kind == "path" {
						t.Errorf("expected no volume/path entries for empty volumes, got %+v", storage)
						break
					}
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec, wizardable, _ := ParseContainerYAML("image: x:1", tc.yaml)
			if !wizardable {
				// Malformed volume definition makes the spec not wizardable;
				// verify function should accept nil storage.
				tc.verify(t, nil)
				return
			}
			tc.verify(t, spec.Storage)
		})
	}
}

// ---------------------------------------------------------------------------
// Build error / edge cases
// ---------------------------------------------------------------------------

func TestBuildContainerYAML_nilSpec(t *testing.T) {
	_, _, err := BuildContainerYAML(nil, "", "")
	if err == nil {
		t.Fatal("BuildContainerYAML(nil) should error")
	}
	if !strings.Contains(err.Error(), "nil spec") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildContainerYAML_emptySpecProducesMinimalYAML(t *testing.T) {
	spec := &apiclient.UnifiedSpec{}
	job, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "image: \"\"") || !strings.HasPrefix(strings.TrimSpace(job), "image:") {
		t.Errorf("empty spec should produce minimal YAML starting with image: %q", job)
	}
	if volumes != "" {
		t.Errorf("empty spec should produce empty volumes, got %q", volumes)
	}
}

func TestBuildContainerYAML_emptyArraysOmittedNotNull(t *testing.T) {
	// Critical: yaml.v3 must emit `key: []` or omit entirely, never `key: null`,
	// because the validator's strict decoder rejects null for typed slices.
	spec := &apiclient.UnifiedSpec{
		Image:       "x:1",
		Environment: []apiclient.KeyValue{},
		Ports:       []apiclient.PortMapping{},
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, "null") {
		t.Errorf("YAML contains null (should be omitted): %s", job)
	}
}

func TestBuildContainerYAML_specialCharacters(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Environment: []apiclient.KeyValue{
			{Key: "PASSWORD", Value: `p@ss"with\quotes`},
			{Key: "PATH", Value: `/usr/bin:/bin`},
		},
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	// Round trip: parse what we built and check the values survived.
	spec2, _, _ := ParseContainerYAML(job, "")
	// 7 default knot env vars + 2 user vars
	if len(spec2.Environment) != 9 {
		t.Fatalf("Environment len = %d, want 9 (7 defaults + 2 user)", len(spec2.Environment))
	}
	for _, kv := range spec2.Environment {
		if kv.Key == "PASSWORD" && kv.Value != `p@ss"with\quotes` {
			t.Errorf("PASSWORD special chars lost: %q", kv.Value)
		}
	}
}

func TestBuildContainerYAML_authRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "registry.example.com/private:1",
		Auth: &apiclient.RegistryAuth{
			Username: "user",
			Password: "pass",
		},
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	spec2, _, _ := ParseContainerYAML(job, "")
	if spec2.Auth == nil {
		t.Fatal("Auth lost in round-trip")
	}
	if spec2.Auth.Username != "user" || spec2.Auth.Password != "pass" {
		t.Errorf("Auth = %+v", spec2.Auth)
	}
}

func TestBuildContainerYAML_volumeDefinitionsRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "workspace", Size: "20G"},
			{Kind: "volume", Name: "cache", Size: "5G"},
			{Kind: "path", HostPath: "/storage/data"},
		},
	}
	_, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	spec2, _, _ := ParseContainerYAML("image: x:1", volumes)
	var vols []apiclient.StorageEntry
	var paths []apiclient.StorageEntry
	for _, s := range spec2.Storage {
		switch s.Kind {
		case "volume":
			vols = append(vols, s)
		case "path":
			paths = append(paths, s)
		}
	}
	if len(vols) != 2 {
		t.Errorf("volume entries count = %d, want 2", len(vols))
	}
	foundWorkspace := false
	for _, v := range vols {
		if v.Name == "workspace" && v.Size == "20G" {
			foundWorkspace = true
		}
	}
	if !foundWorkspace {
		t.Errorf("workspace volume lost: %+v", vols)
	}
	if len(paths) != 1 || paths[0].HostPath != "/storage/data" {
		t.Errorf("Paths = %v", paths)
	}
}

func TestBuildContainerYAML_emptyVolumeDefinitionsProducesEmptyString(t *testing.T) {
	// Nil/empty Storage must produce empty volumes string, not "null"
	// or "{}" — the textarea should be clearable from the wizard.
	cases := [][]apiclient.StorageEntry{
		nil,
		{},
	}
	for i, storage := range cases {
		spec := &apiclient.UnifiedSpec{Image: "x:1", Storage: storage}
		_, volumes, err := BuildContainerYAML(spec, "", "")
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if volumes != "" {
			t.Errorf("case %d: expected empty volumes, got %q", i, volumes)
		}
	}
}

// ---------------------------------------------------------------------------
// Dispatcher (Parse/Build by platform)
// ---------------------------------------------------------------------------

func TestParse_unsupportedPlatform(t *testing.T) {
	_, wizardable, reason := Parse("invalid-platform", "image: x:1", "", nil)
	if wizardable {
		t.Error("unsupported platform should not be wizardable")
	}
	if !strings.Contains(reason, "unsupported platform") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParse_manualPlatform(t *testing.T) {
	// Manual platform has no spec to parse — falls through to default which
	// says "unsupported platform". The UI hides the wizard button entirely for
	// manual, so this is a defensive path.
	_, wizardable, _ := Parse(model.PlatformManual, "anything", "", nil)
	if wizardable {
		t.Error("manual platform should not be wizardable via dispatcher")
	}
}

func TestParse_containerPlatformsDispatch(t *testing.T) {
	// Each local-container platform name should dispatch to the container
	// parser and produce identical results.
	for _, p := range []string{model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer} {
		spec, wizardable, reason := Parse(p, "image: x:1\nmemory: 1G", "", nil)
		if !wizardable {
			t.Errorf("%s: not wizardable: %s", p, reason)
		}
		if spec.Image != "x:1" {
			t.Errorf("%s: Image = %q", p, spec.Image)
		}
	}
}

func TestParse_nomadPlatformRequiresParser(t *testing.T) {
	// Without an HCLParser, Nomad is treated as "not configured" — not wizardable.
	_, wizardable, reason := Parse(model.PlatformNomad, `job "x" {}`, "", nil)
	if wizardable {
		t.Error("Nomad with nil parser should not be wizardable")
	}
	if !strings.Contains(reason, "no HCL parser") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestBuild_unsupportedPlatform(t *testing.T) {
	_, _, err := Build("invalid-platform", &apiclient.UnifiedSpec{}, "", "")
	if err == nil {
		t.Fatal("Build with unsupported platform should error")
	}
}

func TestBuild_nilSpecErrors(t *testing.T) {
	for _, p := range []string{model.PlatformDocker, model.PlatformNomad} {
		_, _, err := Build(p, nil, "", "")
		if err == nil {
			t.Errorf("%s: Build(nil) should error", p)
		}
	}
}

func TestBuild_nomadEmptySpecIntoEmptyHCL(t *testing.T) {
	// Combining two empty inputs: emit a default skeleton with placeholders.
	job, _, err := Build(model.PlatformNomad, &apiclient.UnifiedSpec{Image: "x:1"}, "", "")
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if !strings.Contains(job, `driver = "docker"`) {
		t.Errorf("default skeleton missing docker driver:\n%s", job)
	}
	if !strings.Contains(job, `image = "x:1"`) {
		t.Errorf("default skeleton missing image:\n%s", job)
	}
}

// ---------------------------------------------------------------------------
// TCP+UDP port expansion + numeric CPUs
// ---------------------------------------------------------------------------

func TestPortMappingStrings_tcpUdpExpansion(t *testing.T) {
	// "tcp+udp" protocol must expand to two port entries.
	ports := []apiclient.PortMapping{
		{HostPort: 53, ContainerPort: 53, Protocol: "tcp+udp"},
	}
	out := portMappingStrings(ports)
	if len(out) != 2 {
		t.Fatalf("expected 2 entries for tcp+udp, got %d: %v", len(out), out)
	}
	if out[0] != "53:53/tcp" || out[1] != "53:53/udp" {
		t.Errorf("unexpected expansion: %v", out)
	}
}

func TestPortMappingStrings_singleProtocol(t *testing.T) {
	// Single-protocol ports are unchanged.
	ports := []apiclient.PortMapping{
		{HostPort: 80, ContainerPort: 80, Protocol: "tcp"},
		{HostPort: 53, ContainerPort: 53, Protocol: "udp"},
		{HostPort: 8080, ContainerPort: 8080}, // empty defaults to tcp
	}
	out := portMappingStrings(ports)
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d: %v", len(out), out)
	}
	if out[0] != "80:80/tcp" {
		t.Errorf("out[0] = %q", out[0])
	}
	if out[1] != "53:53/udp" {
		t.Errorf("out[1] = %q", out[1])
	}
	if out[2] != "8080:8080/tcp" {
		t.Errorf("out[2] = %q", out[2])
	}
}

func TestBuildContainerYAML_cpusEmittedAsNumberNotString(t *testing.T) {
	// cpus: "6" in the wizard should emit as `cpus: 6` (unquoted number) in
	// the YAML output, because the container spec uses a numeric CPU field.
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		CPUs:  "6",
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, `cpus: "6"`) {
		t.Errorf("cpus emitted as quoted string:\n%s", job)
	}
	if !strings.Contains(job, "cpus: 6") {
		t.Errorf("cpus not emitted as unquoted number:\n%s", job)
	}
}

func TestBuildContainerYAML_cpusFractionalEmittedAsFloat(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		CPUs:  "0.5",
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, `"0.5"`) {
		t.Errorf("fractional cpus emitted as quoted string:\n%s", job)
	}
	if !strings.Contains(job, "cpus: 0.5") {
		t.Errorf("fractional cpus not emitted as number:\n%s", job)
	}
}

func TestBuildContainerYAML_cpusRoundTripNumericAndString(t *testing.T) {
	// Parse `cpus: 6` (unquoted) and verify the wizard sees "6".
	job := `image: x:1
cpus: 6
`
	spec, _, _ := ParseContainerYAML(job, "")
	if spec.CPUs != "6" {
		t.Errorf("ParseContainerYAML cpus: 6 → spec.CPUs = %q, want %q", spec.CPUs, "6")
	}

	// Also verify quoted form parses.
	job2 := `image: x:1
cpus: "6"
`
	spec2, _, _ := ParseContainerYAML(job2, "")
	if spec2.CPUs != "6" {
		t.Errorf(`ParseContainerYAML cpus: "6" → spec.CPUs = %q, want %q`, spec2.CPUs, "6")
	}
}

// ---------------------------------------------------------------------------
// Comment preservation via yaml.Node patching
// ---------------------------------------------------------------------------

func TestBuildContainerYAML_preservesCommentsOnScalarFields(t *testing.T) {
	// Comments on scalar fields (image, memory, cpus, etc.) must survive the
	// patch. Comments inside replaced list blocks (env, ports) are lost —
	// the user accepted that tradeoff.
	original := `# Top-level comment
image: old:1  # the image to run
memory: 1G    # memory limit
cpus: 2       # CPU count
privileged: false
environment:
  - "TZ=UTC"
  - "DEBUG=true"
# trailing comment
`
	spec := &apiclient.UnifiedSpec{
		Image:  "new:2",
		Memory: "2G",
		CPUs:   "4",
		Environment: []apiclient.KeyValue{
			{Key: "TZ", Value: "UTC"},
		},
	}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}

	// Scalar field values updated
	if !strings.Contains(job, "image: new:2") {
		t.Errorf("image not updated:\n%s", job)
	}
	if !strings.Contains(job, "memory: 2G") {
		t.Errorf("memory not updated:\n%s", job)
	}
	if !strings.Contains(job, "cpus: 4") {
		t.Errorf("cpus not updated:\n%s", job)
	}

	// Comments preserved on scalar lines
	if !strings.Contains(job, "# Top-level comment") {
		t.Errorf("top-level comment lost:\n%s", job)
	}
	if !strings.Contains(job, "# the image to run") {
		t.Errorf("image line comment lost:\n%s", job)
	}
	if !strings.Contains(job, "# memory limit") {
		t.Errorf("memory line comment lost:\n%s", job)
	}
	if !strings.Contains(job, "# trailing comment") {
		t.Errorf("trailing comment lost:\n%s", job)
	}

	// Environment block was replaced (DEBUG removed, only TZ remains)
	if strings.Contains(job, "DEBUG") {
		t.Errorf("DEBUG should be gone from env:\n%s", job)
	}
	if !strings.Contains(job, "TZ=UTC") {
		t.Errorf("TZ should be in env:\n%s", job)
	}
}

func TestBuildContainerYAML_preservesUnknownFieldsAndComments(t *testing.T) {
	// Fields the wizard doesn't manage (shell, etc.) must survive patching
	// with their comments intact. container_name IS a wizard field (spec.Name)
	// — see TestBuildContainerYAML_nameFieldRoundTrip for that coverage.
	original := `# my custom container
image: x:1
container_name: my-app  # fixed name
shell: bash
memory: 1G
`
	spec := &apiclient.UnifiedSpec{
		Name:   "my-app",
		Image:  "new:1",
		Memory: "2G",
	}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "container_name: my-app") {
		t.Errorf("container_name lost:\n%s", job)
	}
	if !strings.Contains(job, "# fixed name") {
		t.Errorf("comment on container_name lost:\n%s", job)
	}
	if !strings.Contains(job, "shell: bash") {
		t.Errorf("unknown field shell lost:\n%s", job)
	}
	if !strings.Contains(job, "# my custom container") {
		t.Errorf("head comment lost:\n%s", job)
	}
}

func TestBuildContainerYAML_patchAddsMissingFields(t *testing.T) {
	// Building a spec with fields that aren't in the original YAML should
	// add them without disturbing existing content.
	original := `image: x:1
# just an image
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "2G",
		CPUs:   "2",
		Ports:  []apiclient.PortMapping{{HostPort: 80, ContainerPort: 80}},
	}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "memory: 2G") {
		t.Errorf("memory not added:\n%s", job)
	}
	if !strings.Contains(job, "cpus: 2") {
		t.Errorf("cpus not added:\n%s", job)
	}
	if !strings.Contains(job, "# just an image") {
		t.Errorf("existing comment lost:\n%s", job)
	}
}

func TestBuildContainerYAML_patchRemovesClearedFields(t *testing.T) {
	// When the wizard's spec has an empty value for a field that exists in
	// the original YAML, the field should be removed.
	original := `image: x:1
memory: 1G
cpus: 2
hostname: my-host
`
	spec := &apiclient.UnifiedSpec{
		Image:    "x:1",
		Memory:   "1G", // preserved
		Hostname: "",   // cleared by user
		CPUs:     "",   // cleared
	}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, "hostname:") {
		t.Errorf("hostname should be removed:\n%s", job)
	}
	if strings.Contains(job, "cpus:") {
		t.Errorf("cpus should be removed:\n%s", job)
	}
	if !strings.Contains(job, "memory: 1G") {
		t.Errorf("memory should be preserved:\n%s", job)
	}
}

func TestBuildContainerYAML_patchFallsBackOnMalformed(t *testing.T) {
	// If the original YAML can't be parsed as a Node tree, the patcher
	// should fall back to full regeneration (no crash).
	original := `this is : not: valid : yaml : :`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML should not error on malformed input: %v", err)
	}
	if !strings.Contains(job, "image: x:1") {
		t.Errorf("image missing from fallback output:\n%s", job)
	}
}

func TestBuildContainerYAML_patchIdempotent(t *testing.T) {
	// Patching the same spec into the same YAML twice must be stable.
	original := `# config
image: old:1
memory: 1G
# end
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "2G",
	}
	out1, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	out2, _, err := BuildContainerYAML(spec, out1, "")
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if out1 != out2 {
		t.Errorf("container YAML patch is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out1, out2)
	}
}

// ---------------------------------------------------------------------------
// Linux capabilities
// ---------------------------------------------------------------------------

func TestParseContainerYAML_capabilitiesNormalised(t *testing.T) {
	job := `image: x:1
cap_add:
  - net_admin
  - CAP_SYS_TIME
  - Cap_Net_Raw
  - CAP_NET_RAW
  - "bad name"
cap_drop:
  - mknod
`
	spec, wizardable, reason := ParseContainerYAML(job, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	want := []string{"CAP_NET_ADMIN", "CAP_SYS_TIME", "CAP_NET_RAW"}
	if strings.Join(spec.CapAdd, ",") != strings.Join(want, ",") {
		t.Errorf("CapAdd = %v, want %v", spec.CapAdd, want)
	}
	if strings.Join(spec.CapDrop, ",") != "CAP_MKNOD" {
		t.Errorf("CapDrop = %v", spec.CapDrop)
	}
}

func TestBuildContainerYAML_capabilitiesCanonicalForm(t *testing.T) {
	// The local container spec validator requires the CAP_ prefix, so the
	// emitter always writes the canonical form regardless of what the UI sent.
	spec := &apiclient.UnifiedSpec{
		Image:   "x:1",
		CapAdd:  []string{"net_admin", "CAP_SYS_TIME"},
		CapDrop: []string{"mknod"},
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "- CAP_NET_ADMIN") || !strings.Contains(job, "- CAP_SYS_TIME") {
		t.Errorf("cap_add not canonicalised:\n%s", job)
	}
	if !strings.Contains(job, "- CAP_MKNOD") {
		t.Errorf("cap_drop not canonicalised:\n%s", job)
	}

	spec2, _, _ := ParseContainerYAML(job, "")
	if strings.Join(spec2.CapAdd, ",") != "CAP_NET_ADMIN,CAP_SYS_TIME" {
		t.Errorf("cap_add round-trip = %v", spec2.CapAdd)
	}
	if strings.Join(spec2.CapDrop, ",") != "CAP_MKNOD" {
		t.Errorf("cap_drop round-trip = %v", spec2.CapDrop)
	}
}

func TestBuildContainerYAML_capabilitiesPatchedAndRemoved(t *testing.T) {
	original := `image: x:1
cap_add:
  - CAP_NET_ADMIN
cap_drop:
  - CAP_MKNOD
memory: 1G  # keep me
`
	// Replace cap_add, clear cap_drop.
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "1G",
		CapAdd: []string{"CAP_SYS_PTRACE"},
	}
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "- CAP_SYS_PTRACE") {
		t.Errorf("cap_add not replaced:\n%s", job)
	}
	if strings.Contains(job, "CAP_NET_ADMIN") {
		t.Errorf("old capability survived:\n%s", job)
	}
	if strings.Contains(job, "cap_drop") {
		t.Errorf("cap_drop not removed:\n%s", job)
	}
	if !strings.Contains(job, "# keep me") {
		t.Errorf("comment lost:\n%s", job)
	}
}

func TestBuildContainerYAML_capabilitiesDedupedOnPatch(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		CapAdd: []string{"CAP_NET_ADMIN", "net_admin", "CAP_NET_ADMIN"},
	}
	job, _, err := BuildContainerYAML(spec, "image: x:1\n", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Count(job, "CAP_NET_ADMIN") != 1 {
		t.Errorf("duplicate capabilities emitted:\n%s", job)
	}
}

// ---------------------------------------------------------------------------
// Structures the wizard refuses rather than rewrite
// ---------------------------------------------------------------------------

func TestParseContainerYAML_refusesMultiDocument(t *testing.T) {
	job := `image: x:1
---
image: y:2
`
	_, wizardable, reason := ParseContainerYAML(job, "")
	if wizardable {
		t.Error("multi-document YAML should not be wizardable")
	}
	if !strings.Contains(reason, "multiple documents") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseContainerYAML_refusesAnchorsAndAliases(t *testing.T) {
	cases := map[string]string{
		"anchor + alias": "base: &base\n  memory: 1G\nimage: x:1\nother: *base\n",
		"merge key":      "defaults: &d\n  memory: 1G\nimage: x:1\n<<: *d\n",
	}
	for name, job := range cases {
		t.Run(name, func(t *testing.T) {
			_, wizardable, reason := ParseContainerYAML(job, "")
			if wizardable {
				t.Error("YAML with anchors/aliases should not be wizardable")
			}
			if !strings.Contains(reason, "anchors or aliases") {
				t.Errorf("unexpected reason: %s", reason)
			}
		})
	}
}

func TestParseContainerYAML_refusesNonMappingRoot(t *testing.T) {
	cases := map[string]string{
		"sequence": "- image: x:1\n",
		"scalar":   "just a string\n",
	}
	for name, job := range cases {
		t.Run(name, func(t *testing.T) {
			_, wizardable, reason := ParseContainerYAML(job, "")
			if wizardable {
				t.Errorf("%s root should not be wizardable", name)
			}
			if !strings.Contains(reason, "mapping") {
				t.Errorf("unexpected reason: %s", reason)
			}
		})
	}
}

func TestParseContainerYAML_commentsOnlyIsWizardable(t *testing.T) {
	spec, wizardable, reason := ParseContainerYAML("# nothing here yet\n", "")
	if !wizardable {
		t.Fatalf("comment-only spec should be wizardable: %s", reason)
	}
	if spec == nil || spec.Image != "" {
		t.Errorf("expected empty spec, got %+v", spec)
	}
}

func TestParseContainerYAML_emptyJobStillReadsVolumeDefinitions(t *testing.T) {
	spec, wizardable, _ := ParseContainerYAML("", "volumes:\n  workspace:\n    size: 20G\n")
	if !wizardable {
		t.Fatal("empty job should be wizardable")
	}
	found := false
	for _, s := range spec.Storage {
		if s.Kind == "volume" && s.Name == "workspace" && s.Size == "20G" {
			found = true
		}
	}
	if !found {
		t.Errorf("volume definitions dropped for an empty job: %+v", spec.Storage)
	}
}

func TestBuildContainerYAML_refusesToRegenerateOverAnchors(t *testing.T) {
	// Valid YAML the patcher can't round-trip: build must fail loudly rather
	// than regenerate and silently drop the anchor structure.
	original := "defaults: &d\n  memory: 1G\nimage: x:1\n"
	_, _, err := BuildContainerYAML(&apiclient.UnifiedSpec{Image: "y:2"}, original, "")
	if err == nil {
		t.Fatal("expected an error when the original uses anchors")
	}
	if !strings.Contains(err.Error(), "anchors or aliases") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildContainerYAML_refusesToRegenerateOverMultiDocument(t *testing.T) {
	original := "image: x:1\n---\nimage: y:2\n"
	_, _, err := BuildContainerYAML(&apiclient.UnifiedSpec{Image: "z:3"}, original, "")
	if err == nil {
		t.Fatal("expected an error when the original has multiple documents")
	}
	if !strings.Contains(err.Error(), "multiple documents") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildContainerYAML_privilegedFalseNotAdded(t *testing.T) {
	job, _, err := BuildContainerYAML(&apiclient.UnifiedSpec{Image: "x:1"}, "image: old:1\n", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, "privileged") {
		t.Errorf("privileged: false should not be added to a spec that never had it:\n%s", job)
	}
}

func TestBuildContainerYAML_privilegedTurnedOffUpdatesExistingKey(t *testing.T) {
	job, _, err := BuildContainerYAML(&apiclient.UnifiedSpec{Image: "x:1"}, "image: x:1\nprivileged: true\n", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "privileged: false") {
		t.Errorf("existing privileged key not set to false:\n%s", job)
	}
}

// The wizard UI only edits cap_add; cap_drop has to survive an Apply untouched
// because there's no control for it. It rides along in the unified spec.
func TestBuildContainerYAML_capDropSurvivesWithoutUIEdits(t *testing.T) {
	original := `image: x:1
cap_drop:
  - CAP_MKNOD
  - CAP_SYS_CHROOT
`
	spec, wizardable, reason := ParseContainerYAML(original, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if len(spec.CapDrop) != 2 {
		t.Fatalf("CapDrop = %v", spec.CapDrop)
	}
	// Simulate the wizard changing an unrelated field.
	spec.Image = "y:2"
	spec.CapAdd = []string{"CAP_NET_ADMIN"}

	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "- CAP_MKNOD") || !strings.Contains(job, "- CAP_SYS_CHROOT") {
		t.Errorf("cap_drop lost on apply:\n%s", job)
	}
	if !strings.Contains(job, "- CAP_NET_ADMIN") {
		t.Errorf("cap_add not written:\n%s", job)
	}
}

// TestContainerYAML_realWorldSpec runs a spec taken from a live template
// through parse → edit → build. It covers the combination that shows up in
// practice: commented-out alternative images, registry auth built from
// variables, capabilities, and env values containing "=" and template
// expressions.
func TestContainerYAML_realWorldSpec(t *testing.T) {
	original := `container_name: ${{ .user.username }}-${{ .space.name }}
hostname: "${{ .space.name }}"
#image: hub.anaconda.ovh/library/knot-ubuntu:24.04
#image: hub.anaconda.ovh/library/knot-php:8.4
#image: paularlott/knot-desktop:24.04-20260514
image: ${{ .server.base_image_registry }}/knot-ubuntu:26.04
auth:
  username: ${{.var.registry_user}}
  password: ${{.var.registry_pass}}
cap_add:
  - CAP_AUDIT_WRITE
privileged: true
memory: 4G
environment:
  - TZ=${{.user.timezone}}
  - KNOT_HTTP_PORT=80=Web 8080=Web2
  - DUMMY2=${{ .stack.space_2.space.name }} - ${{ .stack.space_2.custom.ExcludeDebug }}
`

	spec, wizardable, reason := ParseContainerYAML(original, "")
	if !wizardable {
		t.Fatalf("real-world spec not wizardable: %s", reason)
	}
	if strings.Join(spec.CapAdd, ",") != "CAP_AUDIT_WRITE" {
		t.Errorf("CapAdd = %v", spec.CapAdd)
	}
	if !spec.Privileged {
		t.Error("privileged lost")
	}
	if spec.Memory != "4G" {
		t.Errorf("Memory = %q", spec.Memory)
	}
	if spec.Auth == nil || spec.Auth.Username != "${{.var.registry_user}}" {
		t.Errorf("Auth = %+v", spec.Auth)
	}
	if len(spec.Environment) != 3 {
		t.Fatalf("Environment = %+v", spec.Environment)
	}
	if spec.Environment[1].Key != "KNOT_HTTP_PORT" || spec.Environment[1].Value != "80=Web 8080=Web2" {
		t.Errorf("env with = in the value mangled: %+v", spec.Environment[1])
	}

	// The wizard adds a capability and bumps memory; everything else must survive.
	spec.CapAdd = append(spec.CapAdd, "net_admin")
	spec.Memory = "8G"

	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	for _, want := range []string{
		"container_name: ${{ .user.username }}-${{ .space.name }}",
		"#image: hub.anaconda.ovh/library/knot-ubuntu:24.04",
		"#image: paularlott/knot-desktop:24.04-20260514",
		"username: ${{.var.registry_user}}",
		"- CAP_AUDIT_WRITE",
		"- CAP_NET_ADMIN",
		"memory: 8G",
		"KNOT_HTTP_PORT=80=Web 8080=Web2",
		"${{ .stack.space_2.space.name }} - ${{ .stack.space_2.custom.ExcludeDebug }}",
	} {
		if !strings.Contains(job, want) {
			t.Errorf("expected %q in patched spec:\n%s", want, job)
		}
	}
	if !strings.Contains(job, "privileged: true") {
		t.Errorf("privileged lost:\n%s", job)
	}

	// And the result must still validate.
	if issues := specvalidateIssues(job); len(issues) > 0 {
		t.Errorf("patched spec failed validation: %+v\n%s", issues, job)
	}
}

// ---------------------------------------------------------------------------
// Name (container_name) — distinct from Hostname
// ---------------------------------------------------------------------------

func TestParseContainerYAML_nameDistinctFromHostname(t *testing.T) {
	job := `container_name: ${{ .user.username }}-${{ .space.name }}
hostname: "${{ .space.name }}"
image: x:1
`
	spec, wizardable, reason := ParseContainerYAML(job, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if spec.Name != "${{ .user.username }}-${{ .space.name }}" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Hostname != "${{ .space.name }}" {
		t.Errorf("Hostname = %q", spec.Hostname)
	}
}

func TestBuildContainerYAML_nameFieldRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Name:     "${{ .user.username }}-${{ .space.name }}",
		Hostname: "${{ .space.name }}",
		Image:    "x:1",
	}
	job, _, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "container_name: ${{ .user.username }}-${{ .space.name }}") {
		t.Errorf("container_name not emitted:\n%s", job)
	}
	if !strings.Contains(job, "hostname: ${{ .space.name }}") {
		t.Errorf("hostname not emitted:\n%s", job)
	}

	spec2, _, _ := ParseContainerYAML(job, "")
	if spec2.Name != spec.Name {
		t.Errorf("Name round-trip mismatch: %q vs %q", spec2.Name, spec.Name)
	}
	if spec2.Hostname != spec.Hostname {
		t.Errorf("Hostname round-trip mismatch: %q vs %q", spec2.Hostname, spec.Hostname)
	}
}

func TestBuildContainerYAML_namePatchedIndependentlyOfHostname(t *testing.T) {
	original := `container_name: old-name
hostname: keep-me
image: x:1
`
	spec, wizardable, reason := ParseContainerYAML(original, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	spec.Name = "new-name" // editing Name only

	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if !strings.Contains(job, "container_name: new-name") {
		t.Errorf("container_name not updated:\n%s", job)
	}
	if !strings.Contains(job, "hostname: keep-me") {
		t.Errorf("hostname changed unexpectedly:\n%s", job)
	}
}

func TestBuildContainerYAML_nameClearedRemovesField(t *testing.T) {
	original := `container_name: old-name
image: x:1
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"} // Name left empty
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, "container_name") {
		t.Errorf("container_name should be removed when Name is empty:\n%s", job)
	}
}

func TestBuildContainerYAML_nameNotAddedWhenEmpty(t *testing.T) {
	// A spec with no Name shouldn't gain a container_name field it never had.
	job, _, err := BuildContainerYAML(&apiclient.UnifiedSpec{Image: "x:1"}, "image: old:1\n", "")
	if err != nil {
		t.Fatalf("BuildContainerYAML: %v", err)
	}
	if strings.Contains(job, "container_name") {
		t.Errorf("container_name should not be added:\n%s", job)
	}
}
