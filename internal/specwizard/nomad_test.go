package specwizard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// fakeHCLParser returns a parser that yields a single-group, single-docker-task
// job shape that matches what Nomad's /v1/jobs/parse produces for the input HCL.
// We only populate the fields the extractor reads; tests focus on parse logic.
func fakeHCLParser(parsed map[string]interface{}) HCLParser {
	return func(string) (map[string]interface{}, error) {
		return parsed, nil
	}
}

func TestParseNomadHCL_empty(t *testing.T) {
	spec, wizardable, _ := ParseNomadHCL("", "", nil)
	if !wizardable {
		t.Error("empty HCL should be wizardable")
	}
	if spec == nil {
		t.Fatal("nil spec")
	}
}

func TestParseNomadHCL_singleDockerTask(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Name": "app",
				"Tasks": []interface{}{
					map[string]interface{}{
						"Name":   "app",
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image":   "${{ .server.base_image_registry }}/knot-ubuntu:26.04",
							"command": []interface{}{"sleep"},
							"args":    []interface{}{"infinity"},
						},
						"Env": map[string]interface{}{
							"TZ":  "UTC",
							"FOO": "bar",
						},
						"Resources": map[string]interface{}{
							"MemoryMB": float64(2048),
							"Cores":    float64(2),
						},
						"VolumeMounts": []interface{}{
							map[string]interface{}{
								"Volume":      "data",
								"Destination": "/data",
								"ReadOnly":    false,
							},
						},
					},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Mode": "bridge",
						"DynamicPorts": []interface{}{
							map[string]interface{}{
								"Label": "http",
								"Value": 80,
							},
						},
					},
				},
			},
		},
	}

	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	if spec.Image != "${{ .server.base_image_registry }}/knot-ubuntu:26.04" {
		t.Errorf("Image = %q", spec.Image)
	}
	if len(spec.Command) != 2 || spec.Command[0] != "sleep" || spec.Command[1] != "infinity" {
		t.Errorf("Command = %v", spec.Command)
	}
	if len(spec.Environment) != 2 {
		t.Errorf("Environment len = %d", len(spec.Environment))
	}
	if spec.Memory != "2G" {
		t.Errorf("Memory = %q (want 2G)", spec.Memory)
	}
	if spec.CPUs != "2" {
		t.Errorf("CPUs = %q (want 2)", spec.CPUs)
	}
	if len(spec.Storage) != 1 || spec.Storage[0].HostPath != "data" || spec.Storage[0].ContainerPath != "/data" {
		t.Errorf("Storage = %+v", spec.Storage)
	}
	if len(spec.Ports) != 1 || spec.Ports[0].ContainerPort != 80 {
		t.Errorf("Ports = %+v", spec.Ports)
	}
}

func TestParseNomadHCL_multipleTasks(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{"Driver": "docker"},
					map[string]interface{}{"Driver": "docker"},
				},
			},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("multi-task job should not be wizardable")
	}
	if !strings.Contains(reason, "multiple tasks") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_volumeLabelDiffersFromSource(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Volumes": map[string]interface{}{
					"home_volume": map[string]interface{}{
						"Name":   "home_volume",
						"Source": "${{.user.username}}-${{.space.id}}-php",
						"Type":   "csi",
					},
				},
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"VolumeMounts": []interface{}{
							map[string]interface{}{
								"Volume":      "home_volume",
								"Destination": "/home",
							},
						},
					},
				},
			},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("label ≠ source should not be wizardable")
	}
	if !strings.Contains(reason, "label differs from source") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_volumeLabelEqualsSource(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Volumes": map[string]interface{}{
					"data": map[string]interface{}{
						"Name":   "data",
						"Source": "data",
						"Type":   "csi",
					},
				},
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"VolumeMounts": []interface{}{
							map[string]interface{}{
								"Volume":      "data",
								"Destination": "/data",
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("label == source should be wizardable: %s", reason)
	}
	if len(spec.Storage) != 1 {
		t.Errorf("expected 1 storage entry, got %d", len(spec.Storage))
	}
}

func TestParseNomadHCL_nonDockerDriver(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{"Driver": "exec"},
				},
			},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("non-docker driver should not be wizardable")
	}
	if !strings.Contains(reason, `not supported`) {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_nilParser(t *testing.T) {
	_, wizardable, reason := ParseNomadHCL(`job "x" {}`, "", nil)
	if wizardable {
		t.Error("nil parser with non-empty HCL should not be wizardable")
	}
	if !strings.Contains(reason, "no HCL parser") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

// TestParseNomadHCL_escapesTemplateVars verifies that ${...} sequences in the
// HCL are escaped to $${...} before being handed to the parser, so Nomad
// returns the original variable references back to the wizard instead of
// trying to evaluate them as HCL2 template sequences.
func TestParseNomadHCL_escapesTemplateVars(t *testing.T) {
	var capturedInput string
	capturingParser := func(hcl string) (map[string]interface{}, error) {
		capturedInput = hcl
		// Echo a minimal job structure back so ParseNomadHCL is happy.
		return map[string]interface{}{
			"TaskGroups": []interface{}{
				map[string]interface{}{
					"Tasks": []interface{}{
						map[string]interface{}{
							"Driver": "docker",
							"Config": map[string]interface{}{
								"image": "${{ .server.base_image_registry }}/knot-ubuntu:26.04",
							},
							"Env": map[string]interface{}{
								"KNOT_SPACEID": "${{.space.id}}",
								"TZ":           "${{ .user.timezone }}",
							},
						},
					},
				},
			},
		}, nil
	}

	spec, wizardable, reason := ParseNomadHCL(`job "${{ .space.name }}" { task "t" { driver = "docker" } }`, "", capturingParser)
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	// Verify the parser saw the escaped form. (Note: "$${{" contains "${{" as a
	// substring, so we assert presence of the escape prefix, not absence of
	// the unescaped form.)
	if !strings.Contains(capturedInput, "$${{") {
		t.Errorf("input passed to parser was not escaped: %q", capturedInput)
	}

	// Verify the wizard sees the ORIGINAL variable references, not empty values.
	if spec.Image != "${{ .server.base_image_registry }}/knot-ubuntu:26.04" {
		t.Errorf("Image lost variable ref: %q", spec.Image)
	}
	foundSpaceID, foundTZ := false, false
	for _, kv := range spec.Environment {
		if kv.Key == "KNOT_SPACEID" {
			foundSpaceID = true
			if kv.Value != "${{.space.id}}" {
				t.Errorf("KNOT_SPACEID lost variable ref: %q", kv.Value)
			}
		}
		if kv.Key == "TZ" {
			foundTZ = true
			if kv.Value != "${{ .user.timezone }}" {
				t.Errorf("TZ lost variable ref: %q", kv.Value)
			}
		}
	}
	if !foundSpaceID || !foundTZ {
		t.Errorf("env vars missing from spec: %+v", spec.Environment)
	}
}

func TestParseNomadHCL_templateElseNotPreBlocked(t *testing.T) {
	// if/else should NOT be pre-blocked — just try to parse. If the commented
	// result is valid HCL, the wizard proceeds; if not, the parse error is
	// the reason. Here both branches set different keys, so it parses fine.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
			},
		},
	}
	hcl := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        ${{ if .X }}
        cap_add = ["NET_RAW"]
        ${{ else }}
        cap_add = []
        ${{ end }}
      }
    }
  }
}
`
	_, wizardable, _ := ParseNomadHCL(hcl, "", fakeHCLParser(parsed))
	// Should be wizardable — the parser accepts the commented-out HCL.
	if !wizardable {
		t.Error("if/else template should not be pre-blocked, should try to parse")
	}
}

func TestParseNomadHCL_commentsTemplateDirectives(t *testing.T) {
	// Standalone Go-template directive lines (${{ if }}, ${{ end }}, etc.)
	// must be commented out before the HCL reaches Nomad's parser, otherwise
	// Nomad rejects them as invalid syntax. Variable refs inside quoted
	// strings must NOT be commented.
	var capturedInput string
	capturingParser := func(hcl string) (map[string]interface{}, error) {
		capturedInput = hcl
		return map[string]interface{}{
			"TaskGroups": []interface{}{
				map[string]interface{}{
					"Tasks": []interface{}{
						map[string]interface{}{
							"Driver": "docker",
							"Config": map[string]interface{}{"image": "x:1"},
						},
					},
				},
			},
		}, nil
	}
	hcl := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "${{.var.registry_url}}/app:1"
        ${{ if .space.first_boot }}
        mount {
          type = "bind"
          target = "/data"
        }
        ${{ end }}
      }
      env {
        ${{ if .space.first_boot }}
        KNOT_DISABLE_TERMINAL = "true"
        ${{ end }}
      }
    }
  }
}
`
	ParseNomadHCL(hcl, "", capturingParser)

	// Directive lines should be commented out (note: escaping adds extra $).
	if !strings.Contains(capturedInput, "# $${{ if .space.first_boot }}") {
		t.Errorf("template directive not commented:\n%s", capturedInput)
	}
	if !strings.Contains(capturedInput, "# $${{ end }}") {
		t.Errorf("template end directive not commented:\n%s", capturedInput)
	}
	// Variable refs inside quoted strings must NOT be commented.
	if !strings.Contains(capturedInput, `image = "$${{.var.registry_url}}`) {
		t.Errorf("variable ref in quoted string should be escaped not commented:\n%s", capturedInput)
	}
}

func TestBuildNomadHCL_empty(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:  "${{ .server.base_image_registry }}/knot-ubuntu:26.04",
		Memory: "2G",
		CPUs:   "1000", // MHz for Nomad default skeleton
		Environment: []apiclient.KeyValue{
			{Key: "TZ", Value: "${{ .user.timezone }}"},
		},
		Ports: []apiclient.PortMapping{
			{HostPort: 80, ContainerPort: 80},
		},
	}
	job, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}

	mustContain := []string{
		`job "${{ .space.name }}-${{ .user.username }}"`,
		`driver = "docker"`,
		`image = "${{ .server.base_image_registry }}/knot-ubuntu:26.04"`,
		`memory = 2048`,
		`cpu = 1000`,
		`TZ = "${{ .user.timezone }}"`,
		`port "http"`,
		`to = 80`,
	}
	for _, s := range mustContain {
		if !strings.Contains(job, s) {
			t.Errorf("output missing %q\n---\n%s", s, job)
		}
	}
}

func TestBuildNomadHCL_patchImage(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "old-image:1"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "new-image:2",
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "old-image") {
		t.Errorf("old image not patched out:\n%s", out)
	}
	if !strings.Contains(out, `image = "new-image:2"`) {
		t.Errorf("new image not in output:\n%s", out)
	}
	// Job/group scaffolding preserved.
	if !strings.Contains(out, `job "demo"`) {
		t.Errorf("job name lost:\n%s", out)
	}
}

func TestBuildNomadHCL_patchResources(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
      }
      resources {
        memory = 512
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "4G",
		CPUs:   "2",
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "memory = 512") {
		t.Errorf("old memory not patched:\n%s", out)
	}
	if !strings.Contains(out, "memory = 4096") {
		t.Errorf("new memory not in output:\n%s", out)
	}
	if !strings.Contains(out, "cpu = 2") {
		t.Errorf("cpu not in output:\n%s", out)
	}
}

func TestBuildNomadHCL_patchEnvBlock(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      env {
        OLD = "yes"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Environment: []apiclient.KeyValue{
			{Key: "NEW", Value: "value"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, `OLD = "yes"`) {
		t.Errorf("old env not replaced:\n%s", out)
	}
	if !strings.Contains(out, `NEW = "value"`) {
		t.Errorf("new env missing:\n%s", out)
	}
}

func TestParseAndBuildNomadHCL_privilegedRoundTrip(t *testing.T) {
	// Mirrors Nomad's parsed JSON shape for a docker task with privileged=true,
	// hostname and auth set inside config {}.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image":      "hub.example.com/library/knot-valkey:8.1.3",
							"hostname":   "${{ .space.name }}",
							"privileged": true,
							"auth": map[string]interface{}{
								"username": "${{.var.registry_user}}",
								"password": "${{.var.registry_pass}}",
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if !spec.Privileged {
		t.Error("Privileged not extracted from config block")
	}
	if spec.Hostname != "${{ .space.name }}" {
		t.Errorf("Hostname = %q", spec.Hostname)
	}
	if spec.Auth == nil {
		t.Fatal("Auth not extracted")
	}
	if spec.Auth.Username != "${{.var.registry_user}}" || spec.Auth.Password != "${{.var.registry_pass}}" {
		t.Errorf("Auth = %+v", spec.Auth)
	}

	// Now patch back into a fresh HCL skeleton and verify the patched text
	// carries privileged, hostname and auth.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "old:1"
      }
    }
  }
}
`
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "privileged = true") {
		t.Errorf("privileged not patched into output:\n%s", out)
	}
	if !strings.Contains(out, `hostname = "${{ .space.name }}"`) {
		t.Errorf("hostname not patched into output:\n%s", out)
	}
	if !strings.Contains(out, `username = "${{.var.registry_user}}"`) {
		t.Errorf("auth.username not patched into output:\n%s", out)
	}
}

// TestParseNomadHCL_portsWithLabel verifies that Nomad network ports are
// extracted with their label and container port (To), not the host port
// (Value, which is 0 for dynamic ports).
func TestParseNomadHCL_portsWithLabel(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Mode": "bridge",
						"Ports": []interface{}{
							map[string]interface{}{
								"Label": "redis_port",
								"To":    float64(6379),
								"Value": float64(0),
							},
							map[string]interface{}{
								"Label": "http",
								"To":    float64(80),
								"Value": float64(0),
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if len(spec.Ports) != 2 {
		t.Fatalf("Ports len = %d, want 2: %+v", len(spec.Ports), spec.Ports)
	}
	// Labels preserved
	if spec.Ports[0].Label != "redis_port" {
		t.Errorf("Ports[0].Label = %q", spec.Ports[0].Label)
	}
	if spec.Ports[0].ContainerPort != 6379 {
		t.Errorf("Ports[0].ContainerPort = %d", spec.Ports[0].ContainerPort)
	}
	if spec.Ports[1].Label != "http" || spec.Ports[1].ContainerPort != 80 {
		t.Errorf("Ports[1] = %+v", spec.Ports[1])
	}
}

// TestBuildNomadHCL_portsRemovedAlsoClearsConfigPorts verifies that when the
// wizard removes all ports, both the network block AND the config block's
// `ports = [...]` line are removed — no orphaned label references.
func TestBuildNomadHCL_portsRemovedAlsoClearsConfigPorts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    network {
      port "redis_port" {
        to = 6379
      }
    }

    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        ports = ["redis_port"]
      }
    }
  }
}
`
	// Empty spec — user removed the port in the wizard.
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "port \"redis_port\"") {
		t.Errorf("network port block not removed:\n%s", out)
	}
	if strings.Contains(out, "ports = ") {
		t.Errorf("orphaned config ports line not removed:\n%s", out)
	}
}

// TestBuildNomadHCL_portsEditedUpdatesConfigPorts verifies that when ports are
// edited (renamed/changed), the config block's ports = [...] line is updated
// to reference the new labels.
func TestBuildNomadHCL_portsEditedUpdatesConfigPorts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    network {
      port "redis_port" {
        to = 6379
      }
    }

    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        ports = ["redis_port"]
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{
			{Label: "http", ContainerPort: 8080, Protocol: "tcp"},
			{Label: "metrics", ContainerPort: 9090, Protocol: "tcp"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "redis_port") {
		t.Errorf("old label not removed:\n%s", out)
	}
	if !strings.Contains(out, `ports = ["http", "metrics"]`) {
		t.Errorf("config ports line not synced with new labels:\n%s", out)
	}
	if !strings.Contains(out, `port "http"`) || !strings.Contains(out, `port "metrics"`) {
		t.Errorf("new port blocks missing:\n%s", out)
	}
}

func TestMemoryFormatting(t *testing.T) {
	cases := []struct {
		mb   int64
		want string
	}{
		{2048, "2G"},
		{1024, "1G"},
		{512, "512M"},
		{1000, "1000M"},
		{0, ""},
	}
	for _, c := range cases {
		if got := formatMemory(c.mb); got != c.want {
			t.Errorf("formatMemory(%d) = %q, want %q", c.mb, got, c.want)
		}
		// Round-trip where possible.
		if c.mb > 0 {
			back, err := memoryToMB(c.want)
			if err != nil || back != c.mb {
				t.Errorf("memoryToMB(%q) = %d, err=%v; want %d", c.want, back, err, c.mb)
			}
		}
	}
}

func TestProtocolLabels(t *testing.T) {
	if labelToProtocol("http") != "tcp" {
		t.Error("http should map to tcp")
	}
	if labelToProtocol("udp") != "udp" {
		t.Error("udp should map to udp")
	}
	if labelToProtocol("dns-udp") != "udp" {
		t.Error("dns-udp should map to udp")
	}
	if protocolToLabel("tcp") != "tcp" {
		t.Error("tcp should round-trip")
	}
}

// ---------------------------------------------------------------------------
// Nomad HCL parse: unhappy / edge cases
// ---------------------------------------------------------------------------

func TestParseNomadHCL_parserReturnsError(t *testing.T) {
	failingParser := func(string) (map[string]interface{}, error) {
		return nil, fmt.Errorf("nomad unreachable: connection refused")
	}
	_, wizardable, reason := ParseNomadHCL(`job "x" {}`, "", failingParser)
	if wizardable {
		t.Error("parser error should not be wizardable")
	}
	if !strings.Contains(reason, "HCL parse failed") {
		t.Errorf("unexpected reason: %s", reason)
	}
	if !strings.Contains(reason, "connection refused") {
		t.Errorf("parser error not surfaced in reason: %s", reason)
	}
}

func TestParseNomadHCL_noTaskGroups(t *testing.T) {
	parsed := map[string]interface{}{"ID": "demo"}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("job with no TaskGroups should not be wizardable")
	}
	if !strings.Contains(reason, "no task groups") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_emptyTaskGroups(t *testing.T) {
	parsed := map[string]interface{}{"TaskGroups": []interface{}{}}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("empty TaskGroups slice should not be wizardable")
	}
	if !strings.Contains(reason, "no task groups") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_multipleGroups(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{"Name": "a", "Tasks": []interface{}{}},
			map[string]interface{}{"Name": "b", "Tasks": []interface{}{}},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("multiple task groups should not be wizardable")
	}
	if !strings.Contains(reason, "multiple task groups") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_groupWithNoTasks(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{"Name": "app"},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("group with no Tasks key should not be wizardable")
	}
	if !strings.Contains(reason, "no tasks") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_emptyTasksArray(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{"Tasks": []interface{}{}},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("empty Tasks array should not be wizardable")
	}
	if !strings.Contains(reason, "no tasks") {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_taskWithMissingDriver(t *testing.T) {
	// Task with no Driver field — should be treated as not supported.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{"Name": "app"},
				},
			},
		},
	}
	_, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if wizardable {
		t.Error("missing driver should not be wizardable")
	}
	if !strings.Contains(reason, `not supported`) {
		t.Errorf("unexpected reason: %s", reason)
	}
}

func TestParseNomadHCL_taskWithEmptyConfig(t *testing.T) {
	// Task with no Config map at all — wizard should still load (image empty).
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{"Driver": "docker"},
				},
			},
		},
	}
	spec, wizardable, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatal("docker task with empty config should still be wizardable")
	}
	if spec.Image != "" {
		t.Errorf("Image should be empty for missing config, got %q", spec.Image)
	}
}

func TestParseNomadHCL_malformedVolumeMount(t *testing.T) {
	// VolumeMounts with missing fields should be skipped, not crash.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"VolumeMounts": []interface{}{
							map[string]interface{}{"Volume": "data"},                     // missing Destination
							map[string]interface{}{"Destination": "/data"},               // missing Volume
							map[string]interface{}{"Volume": "ok", "Destination": "/ok"}, // valid
							"not-a-map",
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if len(spec.Storage) != 1 {
		t.Errorf("Storage len = %d, want 1 (only the valid entry): %+v", len(spec.Storage), spec.Storage)
	}
}

func TestParseNomadHCL_malformedPortEntries(t *testing.T) {
	// Port entries with missing fields should be skipped, not crash.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Ports": []interface{}{
							map[string]interface{}{"Label": "ok", "To": 80},  // valid
							map[string]interface{}{"Label": "no-to"},         // missing To
							map[string]interface{}{"To": 90},                 // missing Label
							map[string]interface{}{"Label": "zero", "To": 0}, // To=0 → skip
							"not-a-map",
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if len(spec.Ports) != 1 {
		t.Errorf("Ports len = %d, want 1 (only the valid entry): %+v", len(spec.Ports), spec.Ports)
	}
	if spec.Ports[0].Label != "ok" || spec.Ports[0].ContainerPort != 80 {
		t.Errorf("Ports[0] = %+v", spec.Ports[0])
	}
}

func TestParseNomadHCL_duplicatePortLabelsDeduped(t *testing.T) {
	// Nomad may list a port in both Ports and DynamicPorts; dedupe by label.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{"Driver": "docker", "Config": map[string]interface{}{"image": "x:1"}},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Ports": []interface{}{
							map[string]interface{}{"Label": "http", "To": 80},
						},
						"DynamicPorts": []interface{}{
							map[string]interface{}{"Label": "http", "To": 80},
						},
					},
				},
			},
		},
	}
	spec, _, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if len(spec.Ports) != 1 {
		t.Errorf("duplicate label not deduped: %+v", spec.Ports)
	}
}

func TestParseNomadHCL_resourcesMissing(t *testing.T) {
	// No Resources map — Memory and CPUs stay empty.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
			},
		},
	}
	spec, wizardable, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatal("missing resources should still be wizardable")
	}
	if spec.Memory != "" || spec.CPUs != "" {
		t.Errorf("Memory/CPUs should be empty for missing resources: %+v", spec)
	}
}

func TestParseNomadHCL_cpuMHzPreserved(t *testing.T) {
	// cpu = 300 (MHz) must be extracted as "300" with CPUType "mhz", NOT
	// converted to 0.3 cores.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"Resources": map[string]interface{}{
							"MemoryMB": float64(1024),
							"CPU":      float64(300),
						},
					},
				},
			},
		},
	}
	spec, wizardable, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatal("should be wizardable")
	}
	if spec.CPUs != "300" {
		t.Errorf("CPUs = %q, want %q (MHz preserved)", spec.CPUs, "300")
	}
	if spec.CPUType != "mhz" {
		t.Errorf("CPUType = %q, want %q", spec.CPUType, "mhz")
	}
}

func TestParseNomadHCL_coresPreserved(t *testing.T) {
	// cores = 2 must be extracted as "2" with CPUType "cores".
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"Resources": map[string]interface{}{
							"MemoryMB": float64(2048),
							"Cores":    float64(2),
						},
					},
				},
			},
		},
	}
	spec, wizardable, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatal("should be wizardable")
	}
	if spec.CPUs != "2" {
		t.Errorf("CPUs = %q, want %q", spec.CPUs, "2")
	}
	if spec.CPUType != "cores" {
		t.Errorf("CPUType = %q, want %q", spec.CPUType, "cores")
	}
}

func TestBuildNomadHCL_cpuMHzEmitted(t *testing.T) {
	// Default CPUType (empty/"mhz") emits `cpu = N`, not `cores = N`.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      resources {
        cpu = 300
        memory = 1024
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", Memory: "1G", CPUs: "500", CPUType: "mhz"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "cpu = 500") {
		t.Errorf("cpu = 500 not emitted:\n%s", out)
	}
	if strings.Contains(out, "cores") {
		t.Errorf("cores should not be emitted for mhz type:\n%s", out)
	}
}

func TestBuildNomadHCL_coresEmitted(t *testing.T) {
	// CPUType "cores" emits `cores = N`.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", Memory: "2G", CPUs: "4", CPUType: "cores"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "cores = 4") {
		t.Errorf("cores = 4 not emitted:\n%s", out)
	}
	if strings.Contains(out, "cpu =") {
		t.Errorf("cpu should not be emitted for cores type:\n%s", out)
	}
}

func TestParseNomadHCL_staticPortExtracted(t *testing.T) {
	// Static ports may appear in ReservedPorts (not just Ports/DynamicPorts).
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Mode": "bridge",
						"ReservedPorts": []interface{}{
							map[string]interface{}{
								"Label": "web",
								"To":    float64(80),
								"Value": float64(8080), // static = 8080
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, _ := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatal("should be wizardable")
	}
	if len(spec.Ports) != 1 {
		t.Fatalf("Ports len = %d, want 1: %+v", len(spec.Ports), spec.Ports)
	}
	p := spec.Ports[0]
	if p.Label != "web" {
		t.Errorf("Label = %q", p.Label)
	}
	if p.ContainerPort != 80 {
		t.Errorf("ContainerPort = %d, want 80", p.ContainerPort)
	}
	if p.HostPort != 8080 {
		t.Errorf("HostPort = %d, want 8080 (static)", p.HostPort)
	}
}

func TestBuildNomadHCL_staticPortEmitted(t *testing.T) {
	// When HostPort > 0, emit `static = N` in the port block.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{
			{Label: "web", HostPort: 8080, ContainerPort: 80, Protocol: "tcp"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "static = 8080") {
		t.Errorf("static = 8080 not emitted:\n%s", out)
	}
	if !strings.Contains(out, "to = 80") {
		t.Errorf("to = 80 not emitted:\n%s", out)
	}
}

func TestBuildNomadHCL_dynamicPortNoStatic(t *testing.T) {
	// When HostPort is 0 (dynamic), do NOT emit `static =`.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{
			{Label: "web", HostPort: 0, ContainerPort: 80, Protocol: "tcp"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "static") {
		t.Errorf("static should not be emitted for dynamic ports:\n%s", out)
	}
	if !strings.Contains(out, "to = 80") {
		t.Errorf("to = 80 not emitted:\n%s", out)
	}
}

func TestBuildNomadHCL_serviceBlocksEmitted(t *testing.T) {
	// Ports should produce matching service {} blocks for Consul discovery.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{
			{Label: "web", ContainerPort: 80, Protocol: "tcp"},
			{Label: "dns", ContainerPort: 53, Protocol: "tcp+udp"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Single-protocol port → one service block
	if !strings.Contains(out, `name = "web"`) || !strings.Contains(out, `port = "web"`) {
		t.Errorf("service block for 'web' missing:\n%s", out)
	}
	// Multi-protocol port → two service blocks (dns-tcp + dns-udp)
	if !strings.Contains(out, `name = "dns-tcp"`) {
		t.Errorf("service block for 'dns-tcp' missing:\n%s", out)
	}
	if !strings.Contains(out, `name = "dns-udp"`) {
		t.Errorf("service block for 'dns-udp' missing:\n%s", out)
	}
}

func TestBuildNomadHCL_serviceBlocksRemovedWithPorts(t *testing.T) {
	// When ports are removed, service blocks must also be removed.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      service {
        name = "old-service"
        port = "old-port"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"} // no ports
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "service {") {
		t.Errorf("service block should be removed when no ports:\n%s", out)
	}
}

func TestBuildNomadHCL_noServiceBlocksWithoutPorts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "service {") {
		t.Errorf("service block should not appear without ports:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Nomad HCL build: error / edge cases
// ---------------------------------------------------------------------------

func TestBuildNomadHCL_nilSpec(t *testing.T) {
	_, _, err := BuildNomadHCL(nil, "", "")
	if err == nil {
		t.Fatal("BuildNomadHCL(nil, ...) should error")
	}
	if !strings.Contains(err.Error(), "nil spec") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBuildNomadHCL_patchIntoHCLWithNoTaskBlock(t *testing.T) {
	// Original HCL has no task block, so there's nothing to patch. Emitting a
	// default skeleton would throw away whatever the user wrote, so the build
	// must fail instead. (Parse already reports such a job as not wizardable,
	// so the UI never gets here unless the HCL changed under the wizard.)
	original := `job "demo" {
  datacenters = ["dc1"]
  meta {
    owner = "platform-team"
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "fallback:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err == nil {
		t.Fatalf("expected an error for HCL with no task block, got:\n%s", out)
	}
	if !strings.Contains(err.Error(), "no task block") {
		t.Errorf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("failed build should not return partial output:\n%s", out)
	}
}

func TestBuildNomadHCL_patchAddsConfigBlockIfMissing(t *testing.T) {
	// Task has no config block; image change must insert one.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "inserted:1", Privileged: true}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `image = "inserted:1"`) {
		t.Errorf("image not inserted into new config block:\n%s", out)
	}
	if !strings.Contains(out, "privileged = true") {
		t.Errorf("privileged not inserted:\n%s", out)
	}
}

func TestBuildNomadHCL_patchAddsResourcesBlockIfMissing(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", Memory: "2G", CPUs: "1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "memory = 2048") || !strings.Contains(out, "cpu = 1") {
		t.Errorf("resources block not inserted:\n%s", out)
	}
}

func TestBuildNomadHCL_patchAddsEnvBlockIfMissing(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Environment: []apiclient.KeyValue{
			{Key: "TZ", Value: "UTC"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `TZ = "UTC"`) {
		t.Errorf("env block not inserted:\n%s", out)
	}
}

func TestBuildNomadHCL_patchAddsNetworkBlockIfMissing(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{
			{Label: "http", ContainerPort: 80, Protocol: "tcp"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `port "http"`) {
		t.Errorf("network block not inserted:\n%s", out)
	}
	if !strings.Contains(out, `ports = ["http"]`) {
		t.Errorf("config ports reference not inserted:\n%s", out)
	}
}

func TestBuildNomadHCL_patchVolumeMounts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "old"
        destination = "/old"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "new", ContainerPath: "/new"},
			{Kind: "volume", Name: "ro-vol", ContainerPath: "/ro", ReadOnly: true},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, `volume      = "old"`) {
		t.Errorf("old volume_mount not replaced:\n%s", out)
	}
	if !strings.Contains(out, `volume      = "new"`) {
		t.Errorf("new volume_mount not added:\n%s", out)
	}
	if !strings.Contains(out, `read_only   = true`) {
		t.Errorf("read_only flag not emitted:\n%s", out)
	}
}

func TestBuildNomadHCL_removeAllVolumeMounts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "old"
        destination = "/old"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "volume_mount") {
		t.Errorf("volume_mount blocks not removed when spec.Storage is empty:\n%s", out)
	}
}

// TestBuildNomadHCL_volumeToBindClearsVolumeDefinitions verifies that
// converting a managed volume to a bind mount removes the now-stale entry from
// the Volume Definition YAML (the separate `volumes:` text), not just from the
// HCL job's group-level volume stanza.
func TestBuildNomadHCL_volumeToBindClearsVolumeDefinitions(t *testing.T) {
	originalVolumes := "volumes:\n  - name: data\n    id: data\n    plugin_id: mkdir\n    type: host\n"

	// Stage 1: a managed-volume entry → the volume definition is emitted.
	specVol := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "data", ContainerPath: "/data", VolumeType: "host", PluginID: "mkdir"},
		},
	}
	if _, vols, err := BuildNomadHCL(specVol, "", originalVolumes); err != nil {
		t.Fatalf("BuildNomadHCL vol: %v", err)
	} else if !strings.Contains(vols, "data") {
		t.Errorf("expected the volume definition to be present, got:\n%s", vols)
	}

	// Stage 2: that same mount is converted to a bind. The volume definition
	// must be cleared (empty) — the old entry must NOT linger.
	specBind := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/host/data", ContainerPath: "/data"},
		},
	}
	_, vols, err := BuildNomadHCL(specBind, "", originalVolumes)
	if err != nil {
		t.Fatalf("BuildNomadHCL bind: %v", err)
	}
	if strings.TrimSpace(vols) != "" {
		t.Errorf("expected volume definitions cleared after volume→bind, got:\n%s", vols)
	}

	// Stage 3: patch an existing job that declares the managed volume. After
	// conversion to bind, the group-level `volume "data" {}` stanza must be
	// removed from the HCL too (not just the volume definitions).
	originalJob := `job "demo" {
  group "app" {
    volume "data" {
      type            = "host"
      source          = "data"
    }
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "data"
        destination = "/data"
      }
    }
  }
}
`
	out, _, err := BuildNomadHCL(specBind, originalJob, originalVolumes)
	if err != nil {
		t.Fatalf("BuildNomadHCL patch: %v", err)
	}
	if strings.Contains(out, `volume "data"`) {
		t.Errorf("group-level volume stanza not removed after volume→bind:\n%s", out)
	}
}

func TestBuildNomadHCL_volumeNamesWithTemplateVars(t *testing.T) {
	original := `job "demo" {
  group "app" {
    volume "test-${{.user.username}}-${{.space.name}}" {
      type            = "host"
      source          = "test-${{.user.username}}-${{.space.name}}"
      attachment_mode = "file-system"
      access_mode     = "single-node-writer"
    }
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "test-${{.user.username}}-${{.space.name}}"
        destination = "/data"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "test-${{.user.username}}-${{.space.name}}", ContainerPath: "/data"},
			{Kind: "volume", Name: "new-vol", ContainerPath: "/new"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Must not have garbled fragments from mis-matched braces.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "-${{") {
			t.Errorf("orphaned template fragment found (brace mis-match):\n%s", out)
			break
		}
	}
	// The volume stanza with template vars should survive intact.
	count := strings.Count(out, `volume "test-${{.user.username}}-${{.space.name}}" {`)
	if count != 1 {
		t.Errorf("expected 1 volume stanza with template vars, got %d:\n%s", count, out)
	}
	// New volume stanza should be present.
	if !strings.Contains(out, `volume "new-vol" {`) {
		t.Errorf("new-vol stanza missing:\n%s", out)
	}
	// Both volume_mount blocks should be present.
	if strings.Count(out, "volume_mount {") != 2 {
		t.Errorf("expected 2 volume_mount blocks, got %d:\n%s", strings.Count(out, "volume_mount {"), out)
	}
}

func TestBuildNomadHCL_bindMountEmitsConfigMount(t *testing.T) {
	// A bind entry must emit a docker-driver `mount {}` block INSIDE config —
	// NOT a task-level volume_mount, and NO group-level volume stanza or Volume
	// Definition. (docker driver; podman uses `volumes = [...]`.)
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "external-vol", ContainerPath: "/shared"},
			{Kind: "bind", HostPath: "logs", ContainerPath: "/logs", ReadOnly: true},
		},
	}
	out, volYAML, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Bind mounts appear as config-level mount {} blocks.
	if !strings.Contains(out, `source   = "external-vol"`) || !strings.Contains(out, `target   = "/shared"`) {
		t.Errorf("bind mount external-vol → /shared missing from config:\n%s", out)
	}
	if !strings.Contains(out, `source   = "logs"`) || !strings.Contains(out, `readonly = true`) {
		t.Errorf("read-only bind mount missing from config:\n%s", out)
	}
	// NO task-level volume_mount for binds.
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("bind entries must not emit volume_mount:\n%s", out)
	}
	// NO group-level volume stanza.
	if strings.Contains(out, `volume "external-vol" {`) {
		t.Errorf("bind entry should NOT create a volume stanza:\n%s", out)
	}
	if strings.Contains(out, `volume "logs" {`) {
		t.Errorf("bind entry should NOT create a volume stanza:\n%s", out)
	}
	// NO Volume Definition entries for bind entries.
	if strings.TrimSpace(volYAML) != "" {
		t.Errorf("bind entries should NOT produce Volume Definitions:\n%s", volYAML)
	}
}

func TestBuildNomadHCL_bindMountPodmanEmitsVolumes(t *testing.T) {
	// podman driver: bind entries emit `volumes = [...]` inside config — not
	// volume_mount and not docker-style `mount {}` blocks.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "podman"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Driver: "podman",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/host/shared", ContainerPath: "/shared"},
			{Kind: "bind", HostPath: "/host/logs", ContainerPath: "/logs", ReadOnly: true},
		},
	}
	out, volYAML, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `volumes = [`) {
		t.Errorf("podman volumes list missing:\n%s", out)
	}
	if !strings.Contains(out, `"/host/shared:/shared"`) {
		t.Errorf("bind volume entry missing:\n%s", out)
	}
	if !strings.Contains(out, `"/host/logs:/logs:ro"`) {
		t.Errorf("read-only bind volume entry missing:\n%s", out)
	}
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("podman bind must not emit volume_mount:\n%s", out)
	}
	if strings.Contains(out, `type     = "bind"`) {
		t.Errorf("podman bind must not emit docker mount {} blocks:\n%s", out)
	}
	if strings.TrimSpace(volYAML) != "" {
		t.Errorf("bind must not produce Volume Definitions:\n%s", volYAML)
	}
}

func TestBuildNomadHCL_switchDriverDockerToPodman(t *testing.T) {
	// Editing a docker job and switching the driver to podman via the wizard:
	// the task-level `driver =` line must flip AND config mounts must convert
	// from `mount {}` blocks to `volumes = [...]`.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        mount {
          type   = "bind"
          source = "/host/shared"
          target = "/shared"
        }
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Driver: "podman",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/host/shared", ContainerPath: "/shared"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `driver = "podman"`) {
		t.Errorf("driver line should be podman:\n%s", out)
	}
	if strings.Contains(out, `driver = "docker"`) {
		t.Errorf("stale docker driver line present:\n%s", out)
	}
	if !strings.Contains(out, `volumes = [`) {
		t.Errorf("mounts should have converted to podman volumes list:\n%s", out)
	}
	if strings.Contains(out, "mount {") {
		t.Errorf("docker mount {} block should be gone after switch:\n%s", out)
	}
}

func TestBuildNomadHCL_switchDriverPodmanToDocker(t *testing.T) {
	// Reverse switch: podman job edited to docker.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "podman"
      config {
        image   = "x:1"
        volumes = ["/host/shared:/shared"]
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Driver: "docker",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/host/shared", ContainerPath: "/shared"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `driver = "docker"`) {
		t.Errorf("driver line should be docker:\n%s", out)
	}
	if strings.Contains(out, `driver = "podman"`) {
		t.Errorf("stale podman driver line present:\n%s", out)
	}
	if !strings.Contains(out, "mount {") {
		t.Errorf("volumes should have converted to docker mount {} block:\n%s", out)
	}
	if strings.Contains(out, `volumes = [`) {
		t.Errorf("stale podman volumes list present after switch:\n%s", out)
	}
}

func TestParseNomadHCL_configBindMountBecomesStorage(t *testing.T) {
	// A docker config `mount {}` with a host-path source (not local/) parses to
	// a kind=bind storage entry — binds live in the config block, not in
	// task-level volume_mount.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image": "x:1",
							"Mounts": []interface{}{
								map[string]interface{}{
									"Type":     "bind",
									"Source":   "/host/data",
									"Target":   "/data",
									"ReadOnly": true,
								},
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	var bind *apiclient.StorageEntry
	for i := range spec.Storage {
		if spec.Storage[i].Kind == "bind" {
			bind = &spec.Storage[i]
			break
		}
	}
	if bind == nil {
		t.Fatalf("expected a bind storage entry, got %+v", spec.Storage)
	}
	if bind.HostPath != "/host/data" || bind.ContainerPath != "/data" || !bind.ReadOnly {
		t.Errorf("unexpected bind entry: %+v", bind)
	}
}

// TestParseNomadHCL_configBindMountRealisticKeys verifies the docker config
// mount block is parsed when Nomad's jobs/parse API returns it with lowercase
// HCL keys ("mount"/"source"/"target"/"readonly"), which is what real Nomad
// emits (the older PascalCase form is covered by the test above).
func TestParseNomadHCL_configBindMountRealisticKeys(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image": "x:1",
							"mount": []interface{}{
								map[string]interface{}{
									"type":     "bind",
									"source":   "/wef",
									"target":   "/home",
									"readonly": true,
								},
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	var bind *apiclient.StorageEntry
	for i := range spec.Storage {
		if spec.Storage[i].Kind == "bind" {
			bind = &spec.Storage[i]
			break
		}
	}
	if bind == nil {
		t.Fatalf("expected a bind storage entry from the config mount, got %+v", spec.Storage)
	}
	if bind.HostPath != "/wef" || bind.ContainerPath != "/home" || !bind.ReadOnly {
		t.Errorf("unexpected bind entry: %+v", bind)
	}
}

func TestParseBuildNomadHCL_configBindMountRoundTrip(t *testing.T) {
	// A docker config bind mount survives a parse → build round trip and is
	// emitted back as a config-level mount {} (not a volume_mount).
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image": "x:1",
							"Mounts": []interface{}{
								map[string]interface{}{"Type": "bind", "Source": "/host/data", "Target": "/data"},
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `source   = "/host/data"`) || !strings.Contains(out, `target   = "/data"`) {
		t.Errorf("bind not round-tripped through parse→build:\n%s", out)
	}
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("bind should be a config mount, not a volume_mount:\n%s", out)
	}
}

func TestBuildNomadHCL_bindAndTemplateBothInConfig(t *testing.T) {
	// A spec with both a template mount and a bind mount must emit BOTH in the
	// config block — adding a template must not drop an existing bind mount.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/host/data", ContainerPath: "/data"},
		},
		Templates: []apiclient.NomadTemplate{
			{Destination: "local/cfg", Data: "k=v\n", MountTarget: "/etc/cfg"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `source   = "/host/data"`) || !strings.Contains(out, `target   = "/data"`) {
		t.Errorf("bind mount missing from config:\n%s", out)
	}
	if !strings.Contains(out, `source   = "local/cfg"`) || !strings.Contains(out, `target   = "/etc/cfg"`) {
		t.Errorf("template mount missing from config:\n%s", out)
	}
}

func TestBuildNomadHCL_bindAndVolumeShareStanza(t *testing.T) {
	// A "volume" entry creates the stanza + definition + a task volume_mount; a
	// "bind" entry emits a config mount{} (docker) — it no longer shares the
	// volume_mount mechanism and must NOT create a duplicate stanza or def.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "data", ContainerPath: "/data"},
			{Kind: "bind", HostPath: "data", ContainerPath: "/app/data"},
		},
	}
	out, volYAML, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Exactly one volume stanza (from the "volume" entry).
	count := strings.Count(out, `volume "data" {`)
	if count != 1 {
		t.Errorf("expected 1 volume stanza, got %d:\n%s", count, out)
	}
	// Exactly one task-level volume_mount (the "volume" entry); the bind no
	// longer emits one.
	if strings.Count(out, "volume_mount {") != 1 {
		t.Errorf("expected 1 volume_mount, got %d:\n%s", strings.Count(out, "volume_mount {"), out)
	}
	// The bind entry is emitted as a config-level mount {} instead.
	if !strings.Contains(out, `source   = "data"`) || !strings.Contains(out, `target   = "/app/data"`) {
		t.Errorf("bind entry missing from config:\n%s", out)
	}
	// Only one Volume Definition entry (no duplicate from bind).
	defCount := strings.Count(volYAML, "name: data")
	if defCount != 1 {
		t.Errorf("expected 1 Volume Definition entry, got %d:\n%s", defCount, volYAML)
	}
}

func TestBuildNomadHCL_bindMountRoundTrip(t *testing.T) {
	// Parse an HCL that has a volume_mount with no matching Volume Definition.
	// It should be parsed as Kind "bind" and rebuilt as just a volume_mount —
	// no extra volume stanza or Volume Definition.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
						"VolumeMounts": []interface{}{
							map[string]interface{}{
								"Volume":      "ext-data",
								"Destination": "/data",
							},
						},
					},
				},
			},
		},
	}
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "ext-data"
        destination = "/data"
      }
    }
  }
}
`
	spec, wizardable, reason := ParseNomadHCL(original, "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	// Verify it was parsed as a bind entry.
	if len(spec.Storage) != 1 {
		t.Fatalf("expected 1 storage entry, got %d: %+v", len(spec.Storage), spec.Storage)
	}
	if spec.Storage[0].Kind != "bind" {
		t.Errorf("expected Kind=bind, got %s", spec.Storage[0].Kind)
	}
	if spec.Storage[0].HostPath != "ext-data" {
		t.Errorf("expected HostPath=ext-data, got %s", spec.Storage[0].HostPath)
	}
	// Build back.
	out, volYAML, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// The legacy volume_mount is migrated into a config-level mount {} (docker
	// driver): the bind now lives in the config block, not as a volume_mount.
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("legacy volume_mount should be migrated into config:\n%s", out)
	}
	if !strings.Contains(out, `source   = "ext-data"`) || !strings.Contains(out, `target   = "/data"`) {
		t.Errorf("bind not emitted as config mount:\n%s", out)
	}
	// No volume stanza should have been added.
	if strings.Contains(out, `volume "ext-data" {`) {
		t.Errorf("should NOT create volume stanza for bind:\n%s", out)
	}
	// No Volume Definition should have been added.
	if volYAML != "" {
		t.Errorf("should NOT produce Volume Definition YAML for bind, got:\n%s", volYAML)
	}
}

func TestBuildNomadHCL_bindMountRemovedWhenStorageCleared(t *testing.T) {
	// If the spec has no storage entries, existing volume_mount blocks should
	// be removed (whether they were binds or managed volumes).
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "ext-data"
        destination = "/data"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "volume_mount") {
		t.Errorf("volume_mount should be removed when spec.Storage is empty:\n%s", out)
	}
}

func TestBuildNomadHCL_volumeTypeAndAccessMode(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{
				Kind: "volume", Name: "shared", ContainerPath: "/shared",
				VolumeType: "csi",
				AccessModes: []apiclient.VolumeCapability{
					{AccessMode: "multi-node-multi-writer", AttachmentMode: "file-system"},
				},
			},
			{
				Kind: "volume", Name: "local", ContainerPath: "/local",
				VolumeType: "csi",
			},
			{
				Kind: "volume", Name: "hostvol", ContainerPath: "/host",
				VolumeType: "host", PluginID: "mkdir",
			},
		},
	}
	out, volYAML, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// CSI multi-node: access_mode emitted in HCL.
	if !strings.Contains(out, `access_mode     = "multi-node-multi-writer"`) {
		t.Errorf("multi-node access mode not emitted in HCL:\n%s", out)
	}
	// CSI single-node (default): access_mode emitted in HCL.
	if !strings.Contains(out, `access_mode     = "single-node-writer"`) {
		t.Errorf("default single-node access mode not emitted in HCL:\n%s", out)
	}
	// Host: NO attachment_mode or access_mode in HCL stanza.
	if strings.Contains(out, `volume "hostvol" {`) {
		hostSection := out[strings.Index(out, `volume "hostvol" {`):]
		endIdx := strings.Index(hostSection, "}")
		if endIdx > 0 {
			hostSection = hostSection[:endIdx]
		}
		if strings.Contains(hostSection, "attachment_mode") {
			t.Errorf("host volume should NOT have attachment_mode in HCL:\n%s", out)
		}
		if strings.Contains(hostSection, "access_mode") {
			t.Errorf("host volume should NOT have access_mode in HCL:\n%s", out)
		}
	} else {
		t.Errorf("host volume stanza missing:\n%s", out)
	}
	// CSI capabilities in Volume Definition YAML.
	if !strings.Contains(volYAML, "multi-node-multi-writer") {
		t.Errorf("multi-node access mode not in volume definitions:\n%s", volYAML)
	}
	if !strings.Contains(volYAML, "single-node-writer") {
		t.Errorf("default access mode not in volume definitions:\n%s", volYAML)
	}
	// Host volume: NO capabilities injected in Volume Definition YAML.
	if strings.Contains(volYAML, "hostvol") {
		// Check the hostvol section doesn't have capabilities
		hostIdx := strings.Index(volYAML, "hostvol")
		hostSection := volYAML[hostIdx:]
		nextVolIdx := strings.Index(hostSection[1:], "\n- ")
		if nextVolIdx > 0 {
			hostSection = hostSection[:nextVolIdx+1]
		}
		if strings.Contains(hostSection, "single-node-writer") {
			t.Errorf("host volume should NOT have injected capabilities:\n%s", volYAML)
		}
	}
}

func TestBuildNomadHCL_idempotent(t *testing.T) {
	// Patching the same spec into the same HCL twice must give identical
	// output. Non-idempotent patching would cause drift on every wizard Apply.
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "2G",
		CPUs:   "1",
		Environment: []apiclient.KeyValue{
			{Key: "TZ", Value: "UTC"},
		},
		Ports: []apiclient.PortMapping{
			{Label: "http", ContainerPort: 80, Protocol: "tcp"},
		},
		Privileged: true,
	}
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "old:1" }
    }
  }
}
`
	out1, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("first BuildNomadHCL: %v", err)
	}
	out2, _, err := BuildNomadHCL(spec, out1, "")
	if err != nil {
		t.Fatalf("second BuildNomadHCL: %v", err)
	}
	if out1 != out2 {
		t.Errorf("patch is not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out1, out2)
	}
}

func TestBuildNomadHCL_preservesCommentsOutsidePatchedBlocks(t *testing.T) {
	// The user's hand-written HCL often has comments inside the job/group
	// stanzas but OUTSIDE the wizard-controlled blocks. Those must survive.
	original := `# Top-level comment
job "demo" {
  # Job-level comment
  datacenters = ["dc1"]

  group "app" {
    # Group comment
    count = 1

    task "app" {
      # Task comment
      driver = "docker"
      config {
        image = "old:1"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "new:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	preserved := []string{
		"# Top-level comment",
		"# Job-level comment",
		"# Group comment",
		"# Task comment",
		`datacenters = ["dc1"]`,
		"count = 1",
	}
	for _, s := range preserved {
		if !strings.Contains(out, s) {
			t.Errorf("preserved content lost: %q\n---\n%s", s, out)
		}
	}
	if !strings.Contains(out, `image = "new:1"`) {
		t.Errorf("image not patched:\n%s", out)
	}
}

func TestBuildNomadHCL_hclKeyQuoting(t *testing.T) {
	// Env var keys with non-identifier characters must be quoted in HCL.
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Environment: []apiclient.KeyValue{
			{Key: "123_START_WITH_DIGIT", Value: "v"},
			{Key: "has-dash", Value: "v"},
			{Key: "has space", Value: "v"},
			{Key: "VALID_KEY", Value: "v"},
		},
	}
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// The first three should be quoted, the last should be bare.
	if !strings.Contains(out, `"123_START_WITH_DIGIT"`) {
		t.Errorf("digit-leading key not quoted:\n%s", out)
	}
	if !strings.Contains(out, `"has space"`) {
		t.Errorf("space-containing key not quoted:\n%s", out)
	}
	if !strings.Contains(out, "VALID_KEY =") {
		t.Errorf("valid identifier key incorrectly quoted:\n%s", out)
	}
}

func TestBuildNomadHCL_emptySpecRemovesAllControlledBlocks(t *testing.T) {
	// Building a spec with only image set, against an HCL that had env,
	// resources, and volume_mounts, should remove those blocks.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      env {
        TZ = "UTC"
      }
      resources {
        memory = 1024
      }
      volume_mount {
        volume      = "data"
        destination = "/data"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "env {") {
		t.Errorf("empty spec should remove env block:\n%s", out)
	}
	if strings.Contains(out, "resources {") {
		t.Errorf("empty spec should remove resources block:\n%s", out)
	}
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("empty spec should remove volume_mount blocks:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Nomad volume definitions
// ---------------------------------------------------------------------------

func TestParseNomadVolumeDefinitions(t *testing.T) {
	cases := map[string]struct {
		yaml   string
		verify func(*testing.T, []apiclient.StorageEntry)
	}{
		"empty": {
			yaml: "",
			verify: func(t *testing.T, entries []apiclient.StorageEntry) {
				if entries != nil {
					t.Errorf("expected nil, got %+v", entries)
				}
			},
		},
		"single csi volume": {
			yaml: `volumes:
  - id: data
    name: data
    type: csi
    plugin_id: hostpath
    capacity_min: 1G
    capacity_max: 10G
    capabilities:
      - access_mode: single-node-writer
        attachment_mode: file-system
`,
			verify: func(t *testing.T, entries []apiclient.StorageEntry) {
				var vols []apiclient.StorageEntry
				for _, e := range entries {
					if e.Kind == "volume" {
						vols = append(vols, e)
					}
				}
				if len(vols) != 1 {
					t.Fatalf("volume entries = %+v", entries)
				}
				v := vols[0]
				if v.Name != "data" || v.PluginID != "hostpath" || v.VolumeType != "csi" {
					t.Errorf("volume fields wrong: %+v", v)
				}
				if v.CapacityMin != "1G" || v.CapacityMax != "10G" {
					t.Errorf("capacity wrong: %+v", v)
				}
				if len(v.AccessModes) != 1 || v.AccessModes[0].AccessMode != "single-node-writer" || v.AccessModes[0].AttachmentMode != "file-system" {
					t.Errorf("capabilities wrong: %+v", v)
				}
			},
		},
		"paths only": {
			yaml: "paths:\n  - /storage/${{ .space.id }}/data\n",
			verify: func(t *testing.T, entries []apiclient.StorageEntry) {
				var paths []apiclient.StorageEntry
				for _, e := range entries {
					if e.Kind == "path" {
						paths = append(paths, e)
					}
				}
				if len(paths) != 1 {
					t.Fatalf("Paths = %v", entries)
				}
				if paths[0].HostPath != "/storage/${{ .space.id }}/data" {
					t.Errorf("template var lost: %q", paths[0].HostPath)
				}
			},
		},
		"malformed": {
			yaml: "volumes: [broken\n",
			verify: func(t *testing.T, entries []apiclient.StorageEntry) {
				if entries != nil {
					t.Errorf("expected nil for malformed yaml, got %+v", entries)
				}
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			entries := parseNomadVolumeDefinitionsToStorage(tc.yaml)
			tc.verify(t, entries)
		})
	}
}

func TestBuildNomadVolumeDefinitionsRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{
				Kind:        "volume",
				Name:        "data",
				VolumeType:  "csi",
				PluginID:    "hostpath",
				CapacityMin: "1G",
				CapacityMax: "10G",
				FsType:      "ext4",
				AccessModes: []apiclient.VolumeCapability{
					{AccessMode: "single-node-writer", AttachmentMode: "file-system"},
				},
			},
			{Kind: "path", HostPath: "/storage/data"},
		},
	}
	original := ""
	job, volumes, err := BuildNomadHCL(spec, "task \"app\" { driver = \"docker\" }", original)
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	_ = job
	if volumes == "" {
		t.Fatal("volumes empty after build")
	}
	// Re-parse and check.
	entries := parseNomadVolumeDefinitionsToStorage(volumes)
	var vols []apiclient.StorageEntry
	for _, e := range entries {
		if e.Kind == "volume" {
			vols = append(vols, e)
		}
	}
	if len(vols) != 1 {
		t.Fatalf("round-trip volume entries = %+v", entries)
	}
	v := vols[0]
	if v.Name != "data" || v.PluginID != "hostpath" {
		t.Errorf("volume fields lost: %+v", v)
	}
	if v.CapacityMin != "1G" || v.CapacityMax != "10G" {
		t.Errorf("capacity lost: %+v", v)
	}
}

func TestBuildNomadHCL_nilStorageClearsVolumeDefinitions(t *testing.T) {
	// If spec has no Storage (user deleted every row), the Volume Definition
	// YAML is cleared — stale entries must not linger in the volume section.
	originalVolumes := "volumes:\n  - name: untouched\n    type: csi\n    plugin_id: hostpath\n"
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	_, volumes, err := BuildNomadHCL(spec, "", originalVolumes)
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if volumes != "" {
		t.Errorf("nil Storage should clear the volume section:\nwant: %q\ngot:  %q", "", volumes)
	}
}

func TestBuildNomadHCL_deleteAllStorageClearsVolumesAndDefinitions(t *testing.T) {
	// Edit flow: a job with a managed volume (group `volume {}` stanza +
	// volume_mount + a Volume Definition entry). The user deletes every storage
	// row in the wizard (spec.Storage empty). Applying must clear ALL volume
	// artefacts — the group stanza, the task volume_mount, AND the Volume
	// Definition YAML. Applies to both docker and podman drivers.
	originalHCL := `job "demo" {
  group "app" {
    volume "data" {
      type            = "csi"
      source          = "data"
      attachment_mode = "file-system"
      access_mode     = "single-node-writer"
    }
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      volume_mount {
        volume      = "data"
        destination = "/data"
      }
    }
  }
}
`
	originalVolumes := "volumes:\n  - name: data\n    type: csi\n    plugin_id: hostpath\n"
	spec := &apiclient.UnifiedSpec{Image: "x:1", Driver: "docker"}
	out, volYAML, err := BuildNomadHCL(spec, originalHCL, originalVolumes)
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, `volume "data"`) || strings.Contains(out, "volume ") {
		t.Errorf("group volume stanza should be removed:\n%s", out)
	}
	if strings.Contains(out, "volume_mount {") {
		t.Errorf("task volume_mount should be removed:\n%s", out)
	}
	if strings.TrimSpace(volYAML) != "" {
		t.Errorf("Volume Definition YAML should be empty:\n%s", volYAML)
	}
}

// TestParseNomadHCL_authFallbackToRawText verifies that when the Nomad parser
// doesn't return auth in Config.auth (which can happen with some Nomad
// versions or when auth is passed differently), the wizard falls back to
// extracting auth from the raw HCL text.
func TestParseNomadHCL_authFallbackToRawText(t *testing.T) {
	// Parser returns Config WITHOUT auth field.
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image": "x:1",
							// Note: NO "auth" key here.
						},
					},
				},
			},
		},
	}
	hcl := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        auth {
          username = "${{.var.registry_user}}"
          password = "${{.var.registry_pass}}"
        }
      }
    }
  }
}
`
	spec, wizardable, reason := ParseNomadHCL(hcl, "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if spec.Auth == nil {
		t.Fatal("Auth not extracted via raw-text fallback")
	}
	if spec.Auth.Username != "${{.var.registry_user}}" {
		t.Errorf("Username = %q", spec.Auth.Username)
	}
	if spec.Auth.Password != "${{.var.registry_pass}}" {
		t.Errorf("Password = %q", spec.Auth.Password)
	}
}

// TestBuildNomadHCL_nilAuthPreservesExistingBlock verifies that when spec.Auth
// is nil, the existing auth block in the HCL is NOT removed. The wizard must
// not silently drop registry credentials it doesn't understand.
func TestBuildNomadHCL_nilAuthPreservesExistingBlock(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        auth {
          username = "preserve-me"
          password = "secret"
        }
      }
    }
  }
}
`
	// spec.Auth is nil — wizard didn't parse auth.
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "auth {") {
		t.Errorf("auth block was removed when spec.Auth is nil:\n%s", out)
	}
	if !strings.Contains(out, "preserve-me") {
		t.Errorf("auth username was removed:\n%s", out)
	}
	if !strings.Contains(out, "secret") {
		t.Errorf("auth password was removed:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Linux capabilities (cap_add / cap_drop in the docker driver config block)
// ---------------------------------------------------------------------------

func nomadJobWithConfig(config map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": config,
					},
				},
			},
		},
	}
}

func TestParseNomadHCL_capabilities(t *testing.T) {
	cases := map[string]struct {
		config   map[string]interface{}
		wantAdd  []string
		wantDrop []string
	}{
		"bare lower-case (Nomad docs form)": {
			config: map[string]interface{}{
				"image":    "x:1",
				"cap_add":  []interface{}{"net_admin", "sys_time"},
				"cap_drop": []interface{}{"mknod"},
			},
			wantAdd:  []string{"CAP_NET_ADMIN", "CAP_SYS_TIME"},
			wantDrop: []string{"CAP_MKNOD"},
		},
		"CAP_ prefixed": {
			config: map[string]interface{}{
				"image":   "x:1",
				"cap_add": []interface{}{"CAP_NET_RAW"},
			},
			wantAdd: []string{"CAP_NET_RAW"},
		},
		"mixed case and duplicates": {
			config: map[string]interface{}{
				"image":   "x:1",
				"cap_add": []interface{}{"Net_Admin", "CAP_NET_ADMIN", "net_raw"},
			},
			wantAdd: []string{"CAP_NET_ADMIN", "CAP_NET_RAW"},
		},
		"malformed entries dropped": {
			config: map[string]interface{}{
				"image":   "x:1",
				"cap_add": []interface{}{"net admin", "", "net_raw", 42},
			},
			wantAdd: []string{"CAP_NET_RAW"},
		},
		"absent": {
			config:  map[string]interface{}{"image": "x:1"},
			wantAdd: nil,
		},
		"string slice from a non-JSON source": {
			config: map[string]interface{}{
				"image":   "x:1",
				"cap_add": []string{"sys_ptrace"},
			},
			wantAdd: []string{"CAP_SYS_PTRACE"},
		},
		"wrong type is ignored": {
			config: map[string]interface{}{
				"image":   "x:1",
				"cap_add": 12345,
			},
			wantAdd: nil,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(nomadJobWithConfig(tc.config)))
			if !wizardable {
				t.Fatalf("not wizardable: %s", reason)
			}
			if strings.Join(spec.CapAdd, ",") != strings.Join(tc.wantAdd, ",") {
				t.Errorf("CapAdd = %v, want %v", spec.CapAdd, tc.wantAdd)
			}
			if strings.Join(spec.CapDrop, ",") != strings.Join(tc.wantDrop, ",") {
				t.Errorf("CapDrop = %v, want %v", spec.CapDrop, tc.wantDrop)
			}
		})
	}
}

func TestBuildNomadHCL_capabilitiesEmittedInDefaultSkeleton(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image:   "x:1",
		CapAdd:  []string{"CAP_NET_ADMIN", "CAP_SYS_TIME"},
		CapDrop: []string{"CAP_MKNOD"},
	}
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Fresh specs use the form the Nomad docker driver documents.
	if !strings.Contains(out, `cap_add = ["net_admin", "sys_time"]`) {
		t.Errorf("cap_add not emitted:\n%s", out)
	}
	if !strings.Contains(out, `cap_drop = ["mknod"]`) {
		t.Errorf("cap_drop not emitted:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesInsertedIntoExistingConfig(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image = "x:1"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", CapAdd: []string{"CAP_SYS_PTRACE"}}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `cap_add = ["sys_ptrace"]`) {
		t.Errorf("cap_add not inserted:\n%s", out)
	}
	if !strings.Contains(out, `image = "x:1"`) {
		t.Errorf("image lost:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesReplacedPreservingStyle(t *testing.T) {
	// The original uses the CAP_-prefixed style, so the update should too.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image   = "x:1"
        cap_add = ["CAP_NET_ADMIN"]
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		CapAdd: []string{"CAP_NET_ADMIN", "CAP_NET_RAW"},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `cap_add = ["CAP_NET_ADMIN", "CAP_NET_RAW"]`) {
		t.Errorf("cap_add style not preserved:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesRemovedWhenCleared(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image    = "x:1"
        cap_add  = ["net_admin"]
        cap_drop = ["mknod"]
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "cap_add") {
		t.Errorf("cap_add not removed:\n%s", out)
	}
	if strings.Contains(out, "cap_drop") {
		t.Errorf("cap_drop not removed:\n%s", out)
	}
	if !strings.Contains(out, `image    = "x:1"`) {
		t.Errorf("image line (and its alignment) lost while removing caps:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesMultilineListReplaced(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image = "x:1"
        cap_add = [
          "net_admin",
          "sys_time",
        ]
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", CapAdd: []string{"CAP_NET_RAW"}}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `cap_add = ["net_raw"]`) {
		t.Errorf("multi-line cap_add not replaced:\n%s", out)
	}
	if strings.Contains(out, "sys_time") {
		t.Errorf("old capability survived:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesCompactConfigForm(t *testing.T) {
	// Compact single-line config block: removing a list field must not eat the
	// opening brace.
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1"; cap_add = ["net_admin"] }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "cap_add") {
		t.Errorf("cap_add not removed:\n%s", out)
	}
	if !strings.Contains(out, "config {") {
		t.Errorf("config block opening brace damaged:\n%s", out)
	}
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Errorf("unbalanced braces after patch:\n%s", out)
	}
}

func TestBuildNomadHCL_capabilitiesIdempotent(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image = "x:1"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:   "x:1",
		CapAdd:  []string{"CAP_NET_ADMIN"},
		CapDrop: []string{"CAP_MKNOD"},
	}
	first, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, _, err := BuildNomadHCL(spec, first, "")
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first != second {
		t.Errorf("capability patch not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestNomadCapabilitiesRoundTrip(t *testing.T) {
	// Emit → re-parse (via a fake mirroring Nomad's output) → same canonical set.
	spec := &apiclient.UnifiedSpec{
		Image:   "x:1",
		CapAdd:  []string{"net_admin", "CAP_SYS_TIME"},
		CapDrop: []string{"mknod"},
	}
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `cap_add = ["net_admin", "sys_time"]`) {
		t.Fatalf("unexpected emitted caps:\n%s", out)
	}

	reparsed, wizardable, reason := ParseNomadHCL(out, "", fakeHCLParser(nomadJobWithConfig(map[string]interface{}{
		"image":    "x:1",
		"cap_add":  []interface{}{"net_admin", "sys_time"},
		"cap_drop": []interface{}{"mknod"},
	})))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if strings.Join(reparsed.CapAdd, ",") != "CAP_NET_ADMIN,CAP_SYS_TIME" {
		t.Errorf("CapAdd = %v", reparsed.CapAdd)
	}
	if strings.Join(reparsed.CapDrop, ",") != "CAP_MKNOD" {
		t.Errorf("CapDrop = %v", reparsed.CapDrop)
	}
}

// ---------------------------------------------------------------------------
// Patcher robustness: heredocs, template directives, network mode
// ---------------------------------------------------------------------------

func TestMatchBrace_skipsHeredoc(t *testing.T) {
	// The heredoc body contains unbalanced braces; brace matching must ignore it.
	hcl := `task "app" {
  template {
    data = <<EOH
{{ range service "db" }}
DB_HOST={{ .Address }}
{
EOH
    destination = "local/env"
  }
  driver = "docker"
}
`
	openIdx := strings.Index(hcl, "{")
	closeIdx, ok := matchBrace(hcl, openIdx)
	if !ok {
		t.Fatal("matchBrace failed on heredoc content")
	}
	if closeIdx != len(hcl)-2 { // final "}" before the trailing newline
		t.Errorf("closeIdx = %d, want %d\n%q", closeIdx, len(hcl)-2, hcl[closeIdx:])
	}
}

func TestMatchBrace_skipsIndentedHeredoc(t *testing.T) {
	hcl := `block {
  data = <<-EOF
    {
    unbalanced
    EOF
}
`
	closeIdx, ok := matchBrace(hcl, strings.Index(hcl, "{"))
	if !ok {
		t.Fatal("matchBrace failed on indented heredoc")
	}
	if hcl[closeIdx] != '}' {
		t.Errorf("closeIdx points at %q", hcl[closeIdx])
	}
	if closeIdx != len(hcl)-2 {
		t.Errorf("closeIdx = %d, want %d", closeIdx, len(hcl)-2)
	}
}

func TestBuildNomadHCL_preservesHeredocTemplate(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image = "old:1"
      }

      template {
        data = <<EOH
# generated
{{ with secret "kv/data/app" }}
API_KEY={{ .Data.data.key }}
{{ end }}
EOH
        destination = "secrets/app.env"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "new:2",
		Templates: []apiclient.NomadTemplate{
			{Destination: "secrets/app.env", Data: "# generated\n{{ with secret \"kv/data/app\" }}\nAPI_KEY={{ .Data.data.key }}\n{{ end }}\n"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `image = "new:2"`) {
		t.Errorf("image not patched:\n%s", out)
	}
	if !strings.Contains(out, `{{ with secret "kv/data/app" }}`) {
		t.Errorf("heredoc template body damaged:\n%s", out)
	}
	if !strings.Contains(out, "destination = \"secrets/app.env\"") {
		t.Errorf("template destination lost:\n%s", out)
	}
}

func TestParseNomadHCL_templateBlocks(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image": "x:1",
							"Mounts": []interface{}{
								map[string]interface{}{
									"Type":     "bind",
									"Source":   "local/custom.ini",
									"Target":   "/usr/local/etc/php/conf.d/custom.ini",
									"ReadOnly": false,
								},
								map[string]interface{}{
									"Type":     "bind",
									"Source":   "/cephfs/data",
									"Target":   "/data",
									"ReadOnly": true,
								},
							},
						},
						"Templates": []interface{}{
							map[string]interface{}{
								"Data":        "KEY=value\n",
								"DestPath":    "local/env",
								"ChangeMode":  "noop",
							},
							map[string]interface{}{
								"Data":         "post_max_size = 50M\n",
								"DestPath":     "local/custom.ini",
								"ChangeMode":   "signal",
								"ChangeSignal": "SIGHUP",
							},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if len(spec.Templates) != 2 {
		t.Fatalf("expected 2 templates, got %d: %+v", len(spec.Templates), spec.Templates)
	}
	// First template has no mount.
	t1 := spec.Templates[0]
	if t1.Destination != "local/env" {
		t.Errorf("t1 destination: %s", t1.Destination)
	}
	if t1.Data != "KEY=value\n" {
		t.Errorf("t1 data: %q", t1.Data)
	}
	if t1.MountTarget != "" {
		t.Errorf("t1 should have no mount, got %s", t1.MountTarget)
	}
	// Second template has a mount with matching source.
	t2 := spec.Templates[1]
	if t2.Destination != "local/custom.ini" {
		t.Errorf("t2 destination: %s", t2.Destination)
	}
	if t2.ChangeMode != "signal" {
		t.Errorf("t2 change_mode: %s", t2.ChangeMode)
	}
	if t2.ChangeSignal != "SIGHUP" {
		t.Errorf("t2 change_signal: %s", t2.ChangeSignal)
	}
	if t2.MountTarget != "/usr/local/etc/php/conf.d/custom.ini" {
		t.Errorf("t2 mount target: %s", t2.MountTarget)
	}
}

func TestBuildNomadHCL_templateBlocksWithMounts(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        mount {
          type     = "bind"
          source   = "/cephfs/data"
          target   = "/data"
          readonly = true
        }
        mount {
          type     = "bind"
          source   = "local/old.ini"
          target   = "/old.ini"
          readonly = true
        }
      }
      template {
        data = <<EOF
old data
EOF
        destination = "local/old.ini"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		// The pre-existing /cephfs/data bind mount is in storage (the wizard
		// now owns all config-level mounts: templates + binds), so it is
		// re-emitted rather than dropped on rebuild.
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/cephfs/data", ContainerPath: "/data", ReadOnly: true},
		},
		Templates: []apiclient.NomadTemplate{
			{Destination: "local/custom.ini", Data: "new data\n", ChangeMode: "signal", ChangeSignal: "SIGHUP", MountTarget: "/etc/custom.ini", MountReadonly: true},
			{Destination: "local/env", Data: "KEY=val\n"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// Old template replaced — not present.
	if strings.Contains(out, "local/old.ini") {
		t.Errorf("old template should be replaced:\n%s", out)
	}
	// New templates present.
	if !strings.Contains(out, "destination = \"local/custom.ini\"") {
		t.Errorf("custom.ini template missing:\n%s", out)
	}
	if !strings.Contains(out, "destination = \"local/env\"") {
		t.Errorf("env template missing:\n%s", out)
	}
	// Template data rendered.
	if !strings.Contains(out, "new data") {
		t.Errorf("template data missing:\n%s", out)
	}
	// Change mode/signal emitted.
	if !strings.Contains(out, "change_mode = \"signal\"") {
		t.Errorf("change_mode missing:\n%s", out)
	}
	if !strings.Contains(out, "change_signal = \"SIGHUP\"") {
		t.Errorf("change_signal missing:\n%s", out)
	}
	// Template mount block emitted inside config.
	if !strings.Contains(out, `source   = "local/custom.ini"`) {
		t.Errorf("template mount missing in config:\n%s", out)
	}
	if !strings.Contains(out, `target   = "/etc/custom.ini"`) {
		t.Errorf("template mount target missing:\n%s", out)
	}
	// Non-template mount preserved.
	if !strings.Contains(out, `source   = "/cephfs/data"`) {
		t.Errorf("non-template mount should be preserved:\n%s", out)
	}
	// Old template mount removed.
	if strings.Contains(out, `source   = "local/old.ini"`) {
		t.Errorf("old template mount should be removed:\n%s", out)
	}
}

func TestBuildNomadHCL_emptyTemplatesRemovesBlocks(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image = "x:1"
        mount {
          type   = "bind"
          source = "local/cfg"
          target = "/cfg"
        }
      }
      template {
        data = <<EOF
data
EOF
        destination = "local/cfg"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "template {") {
		t.Errorf("template blocks should be removed:\n%s", out)
	}
	if strings.Contains(out, `source = "local/cfg"`) {
		t.Errorf("template mount should be removed from config:\n%s", out)
	}
}

func TestBuildNomadHCL_templateWithTemplateVarsInData(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Templates: []apiclient.NomadTemplate{
			{Destination: "local/tz.ini", Data: "date.timezone = \"${{.user.timezone}}\"\n"},
		},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "${{.user.timezone}}") {
		t.Errorf("template variable in data should survive:\n%s", out)
	}
}

func TestParseNomadHCL_escapesTemplateDirectives(t *testing.T) {
	// %{ opens an HCL template directive; it must be escaped like ${ or the
	// parser rejects specs that contain it.
	var captured string
	parser := func(hcl string) (map[string]interface{}, error) {
		captured = hcl
		return nomadJobWithConfig(map[string]interface{}{"image": "x:1"}), nil
	}
	_, wizardable, reason := ParseNomadHCL(`job "x" { meta { fmt = "%{ literal }" } }`, "", parser)
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if !strings.Contains(captured, "%%{") {
		t.Errorf("%%{ was not escaped before parsing: %q", captured)
	}
}

func TestParseNomadHCL_networkModePreserved(t *testing.T) {
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{"image": "x:1"},
					},
				},
				"Networks": []interface{}{
					map[string]interface{}{
						"Mode": "host",
						"DynamicPorts": []interface{}{
							map[string]interface{}{"Label": "http", "To": 80},
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL("ignored", "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if spec.Network != "host" {
		t.Fatalf("Network = %q, want host", spec.Network)
	}

	original := `job "demo" {
  group "app" {
    network {
      mode = "host"
      port "http" {
        to = 80
      }
    }

    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `mode = "host"`) {
		t.Errorf("network mode rewritten:\n%s", out)
	}
	if strings.Contains(out, `mode = "bridge"`) {
		t.Errorf("network mode replaced with bridge:\n%s", out)
	}
}

func TestBuildNomadHCL_networkWithoutModeStaysWithoutMode(t *testing.T) {
	// A network block with no mode relies on the driver default; the patcher
	// must not invent one.
	original := `job "demo" {
  group "app" {
    network {
      port "http" {
        to = 80
      }
    }

    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Ports: []apiclient.PortMapping{{Label: "http", ContainerPort: 80, Protocol: "tcp"}},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "mode =") {
		t.Errorf("patcher invented a network mode:\n%s", out)
	}
	if !strings.Contains(out, `port "http"`) {
		t.Errorf("port block lost:\n%s", out)
	}
}

func TestBuildNomadHCL_privilegedFalseNotInserted(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "privileged") {
		t.Errorf("privileged = false should not be written into a spec that never had it:\n%s", out)
	}
}

func TestBuildNomadHCL_privilegedTurnedOffUpdatesExistingLine(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image      = "x:1"
        privileged = true
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", Privileged: false}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "privileged = false") {
		t.Errorf("existing privileged line not updated to false:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Output formatting: indentation of inserted fields and appended blocks
// ---------------------------------------------------------------------------

func TestBuildNomadHCL_insertedConfigFieldsMatchSiblingIndent(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image = "x:1"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:      "x:1",
		Hostname:   "${{ .space.name }}",
		Privileged: true,
		Command:    []string{"sleep", "infinity"},
		CapAdd:     []string{"CAP_NET_ADMIN"},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	for _, want := range []string{
		`        hostname = "${{ .space.name }}"`,
		`        privileged = true`,
		`        args = ["sleep", "infinity"]`,
		`        cap_add = ["net_admin"]`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in output:\n%s", want, out)
		}
	}
}

func TestBuildNomadHCL_appendedBlocksKeepClosingBraceIndent(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Image:  "x:1",
		Memory: "2G",
		Ports:  []apiclient.PortMapping{{Label: "http", ContainerPort: 80}},
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	// The task, group and job closing braces must keep their original columns.
	for _, want := range []string{"\n    }\n", "\n  }\n", "\n}\n"} {
		if !strings.Contains(out, want) {
			t.Errorf("closing brace indentation lost (missing %q):\n%s", want, out)
		}
	}
	if strings.Contains(out, "\n}\n    network") {
		t.Errorf("group body was reflowed:\n%s", out)
	}
	// And the patch must still be idempotent after the append.
	again, _, err := BuildNomadHCL(spec, out, "")
	if err != nil {
		t.Fatalf("second BuildNomadHCL: %v", err)
	}
	if again != out {
		t.Errorf("append path not idempotent:\n--- first ---\n%s\n--- second ---\n%s", out, again)
	}
}

func TestBuildNomadHCL_tabIndentedSpecKeepsTabs(t *testing.T) {
	original := "job \"demo\" {\n\tgroup \"app\" {\n\t\ttask \"app\" {\n\t\t\tdriver = \"docker\"\n\n\t\t\tconfig {\n\t\t\t\timage = \"x:1\"\n\t\t\t}\n\t\t}\n\t}\n}\n"
	spec := &apiclient.UnifiedSpec{Image: "x:1", CapAdd: []string{"CAP_NET_RAW"}}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, "\t\t\t\tcap_add = [\"net_raw\"]") {
		t.Errorf("tab indentation not matched:\n%q", out)
	}
}

func TestBuildNomadHCL_hostnameRemovalKeepsCompactBlockIntact(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { hostname = "old-host"; image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"} // hostname cleared in the wizard
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "hostname") {
		t.Errorf("hostname not removed:\n%s", out)
	}
	if !strings.Contains(out, "config {") {
		t.Errorf("config block opening brace damaged:\n%s", out)
	}
	if strings.Count(out, "{") != strings.Count(out, "}") {
		t.Errorf("unbalanced braces after patch:\n%s", out)
	}
	if !strings.Contains(out, `image = "x:1"`) {
		t.Errorf("image lost:\n%s", out)
	}
}

func TestBuildNomadHCL_hostnameRemovalLeavesNoBlankLine(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image    = "x:1"
        hostname = "old-host"
      }
    }
  }
}
`
	out, _, err := BuildNomadHCL(&apiclient.UnifiedSpec{Image: "x:1"}, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if strings.Contains(out, "hostname") {
		t.Errorf("hostname not removed:\n%s", out)
	}
	if strings.Contains(out, "\"x:1\"\n\n      }") {
		t.Errorf("removal left a blank line:\n%s", out)
	}
}

// cap_drop has no wizard control, so it must round-trip untouched when the user
// edits something else.
func TestBuildNomadHCL_capDropSurvivesWithoutUIEdits(t *testing.T) {
	original := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"

      config {
        image    = "x:1"
        cap_drop = ["mknod", "sys_chroot"]
      }
    }
  }
}
`
	spec, wizardable, reason := ParseNomadHCL(original, "", fakeHCLParser(nomadJobWithConfig(map[string]interface{}{
		"image":    "x:1",
		"cap_drop": []interface{}{"mknod", "sys_chroot"},
	})))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if strings.Join(spec.CapDrop, ",") != "CAP_MKNOD,CAP_SYS_CHROOT" {
		t.Fatalf("CapDrop = %v", spec.CapDrop)
	}

	spec.Image = "y:2"
	spec.CapAdd = []string{"CAP_NET_ADMIN"}

	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `cap_drop = ["mknod", "sys_chroot"]`) {
		t.Errorf("cap_drop lost on apply:\n%s", out)
	}
	if !strings.Contains(out, `cap_add = ["net_admin"]`) {
		t.Errorf("cap_add not written:\n%s", out)
	}
	if !strings.Contains(out, `image    = "y:2"`) {
		t.Errorf("image not patched:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Job label (Name) — distinct from Hostname
// ---------------------------------------------------------------------------

func TestExtractJobLabel(t *testing.T) {
	cases := map[string]string{
		`job "my-job" {`: "my-job",
		`job "${{ .user.username }}-${{ .space.name }}" {`: "${{ .user.username }}-${{ .space.name }}",
		"  job \"indented\" {\n":                           "indented",
		`job   "extra-spaces"   {`:                         "extra-spaces",
		"not a job block at all":                           "",
		`variable "job" { default = "x" }`:                 "",
	}
	for hcl, want := range cases {
		if got := extractJobLabel(hcl); got != want {
			t.Errorf("extractJobLabel(%q) = %q, want %q", hcl, got, want)
		}
	}
}

func TestPatchJobLabel(t *testing.T) {
	hcl := `job "old-name" {
  datacenters = ["dc1"]
}
`
	out := patchJobLabel(hcl, "new-name")
	if !strings.Contains(out, `job "new-name" {`) {
		t.Errorf("label not replaced:\n%s", out)
	}
	if strings.Contains(out, "old-name") {
		t.Errorf("old label survived:\n%s", out)
	}
}

func TestPatchJobLabel_withTemplateVariables(t *testing.T) {
	// Variable syntax is full of "$" — must not be mangled by the
	// replacement-string escaping in ReplaceAllString.
	hcl := `job "plain" {
  datacenters = ["dc1"]
}
`
	out := patchJobLabel(hcl, "${{ .user.username }}-${{ .space.name }}")
	if !strings.Contains(out, `job "${{ .user.username }}-${{ .space.name }}" {`) {
		t.Errorf("variable-laden label mangled:\n%s", out)
	}
}

func TestPatchJobLabel_emptyNameLeavesUnchanged(t *testing.T) {
	hcl := `job "keep-me" {
  datacenters = ["dc1"]
}
`
	out := patchJobLabel(hcl, "")
	if out != hcl {
		t.Errorf("empty name should leave the label untouched:\n%s", out)
	}
}

func TestPatchJobLabel_noJobBlock(t *testing.T) {
	hcl := "not a job at all\n"
	if out := patchJobLabel(hcl, "name"); out != hcl {
		t.Errorf("HCL without a job block should be returned unchanged:\n%s", out)
	}
}

func TestParseNomadHCL_jobLabelDistinctFromHostname(t *testing.T) {
	original := `job "${{.user.username}}-${{.space.name}}" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image    = "x:1"
        hostname = "${{ .space.name }}"
      }
    }
  }
}
`
	parsed := map[string]interface{}{
		"TaskGroups": []interface{}{
			map[string]interface{}{
				"Tasks": []interface{}{
					map[string]interface{}{
						"Driver": "docker",
						"Config": map[string]interface{}{
							"image":    "x:1",
							"hostname": "${{ .space.name }}",
						},
					},
				},
			},
		},
	}
	spec, wizardable, reason := ParseNomadHCL(original, "", fakeHCLParser(parsed))
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}
	if spec.Name != "${{.user.username}}-${{.space.name}}" {
		t.Errorf("Name = %q", spec.Name)
	}
	if spec.Hostname != "${{ .space.name }}" {
		t.Errorf("Hostname = %q", spec.Hostname)
	}
}

func TestBuildNomadHCL_namePatchedIndependentlyOfHostname(t *testing.T) {
	original := `job "old-name" {
  group "app" {
    task "app" {
      driver = "docker"
      config {
        image    = "x:1"
        hostname = "keep-me"
      }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{
		Name:     "new-name",
		Image:    "x:1",
		Hostname: "keep-me",
	}
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `job "new-name" {`) {
		t.Errorf("job label not updated:\n%s", out)
	}
	if !strings.Contains(out, `hostname   = "keep-me"`) && !strings.Contains(out, `hostname = "keep-me"`) {
		t.Errorf("hostname changed unexpectedly:\n%s", out)
	}
}

func TestBuildNomadHCL_nameEmptyLeavesLabelUntouched(t *testing.T) {
	original := `job "existing-name" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1"} // Name left empty
	out, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `job "existing-name" {`) {
		t.Errorf("job label should be left as-is when Name is empty:\n%s", out)
	}
}

func TestBuildNomadHCL_defaultSkeletonUsesCustomName(t *testing.T) {
	spec := &apiclient.UnifiedSpec{Image: "x:1", Name: "my-custom-job"}
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `job "my-custom-job" {`) {
		t.Errorf("custom job name not used in default skeleton:\n%s", out)
	}
}

func TestBuildNomadHCL_defaultSkeletonFallsBackToConventionalName(t *testing.T) {
	spec := &apiclient.UnifiedSpec{Image: "x:1"} // Name left empty
	out, _, err := BuildNomadHCL(spec, "", "")
	if err != nil {
		t.Fatalf("BuildNomadHCL: %v", err)
	}
	if !strings.Contains(out, `job "${{ .space.name }}-${{ .user.username }}" {`) {
		t.Errorf("default job name convention not used:\n%s", out)
	}
}

func TestBuildNomadHCL_nameIdempotent(t *testing.T) {
	original := `job "old" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	spec := &apiclient.UnifiedSpec{Image: "x:1", Name: "${{ .user.username }}-${{ .space.name }}"}
	first, _, err := BuildNomadHCL(spec, original, "")
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	second, _, err := BuildNomadHCL(spec, first, "")
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	if first != second {
		t.Errorf("job label patch not idempotent:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}
