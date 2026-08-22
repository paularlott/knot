package harness

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database/model"
)

// TemplateOptions customises the docker job spec generated for test templates.
type TemplateOptions struct {
	// ExtraEnv entries are appended as KEY=VALUE strings.
	ExtraEnv []string
	// Port, if > 0, declares a named http port on the template.
	PortName string
	Port     uint16
	// Image, when set, overrides the container image for the template
	// (fully qualified reference). Defaults to the harness base image.
	Image string

	// Volumes is the volume definition YAML attached to the template.
	Volumes string
	// AllowMigration enables failed-node migration for the template
	// (AllowNodeMigration + HealthCheckAutoRestart).
	AllowMigration bool
}

// imageRef resolves the template image: an explicit override or the harness
// base image.
func (o TemplateOptions) imageRef(cfg *Config) string {
	if o.Image != "" {
		return o.Image
	}
	return cfg.ImageRef()
}

// TemplateJobYAML renders the docker container spec for a knot-ubuntu space.
// The knot env block mirrors what the spec wizard injects so the in-container
// entrypoint can fetch the agent from the server under test.
func TemplateJobYAML(cfg *Config, opts TemplateOptions) string {
	job := fmt.Sprintf(`image: %s
environment:
  - KNOT_SERVER=${{ .server.url }}
  - KNOT_AGENT_ENDPOINT=${{ .server.agent_endpoint }}
  - KNOT_SPACEID=${{ .space.id }}
  - KNOT_USER=${{ .user.username }}
  - KNOT_LOGLEVEL=info
  - KNOT_USE_TLS=false
  - TZ=${{ .user.timezone }}
`, opts.imageRef(cfg))

	for _, e := range opts.ExtraEnv {
		job += "  - " + e + "\n"
	}

	if opts.Port > 0 {
		// host port 0 = docker assigns an ephemeral port, avoiding
		// collisions when several spaces use the same template.
		job += fmt.Sprintf("ports:\n  - 0:%d\n", opts.Port)
	}
	return job
}

// CreateTemplate creates an active docker-platform template.
func CreateTemplate(s *Server, client *apiclient.ApiClient, name string, opts TemplateOptions) (string, error) {
	ports := []model.TemplatePort{}
	if opts.Port > 0 {
		portName := opts.PortName
		if portName == "" {
			portName = "web"
		}
		ports = append(ports, model.TemplatePort{Name: portName, Port: opts.Port, Protocol: "http"})
	}

	req := &apiclient.TemplateCreateRequest{
		Name:  name,
		Job:   TemplateJobYAML(s.Config, opts),
		Ports: ports,
		CustomFields: []apiclient.CustomFieldDef{
			{Name: "env", Description: "integration test custom field"},
		},
		Volumes:                opts.Volumes,
		Description:            "integration test template",
		Platform:               "container",
		Active:                 true,
		WithTerminal:           true,
		WithSSH:                true,
		WithRunCommand:         true,
		ComputeUnits:           2,
		StorageUnits:           2,
		MaxUptime:              0,
		MaxUptimeUnit:          "disabled",
		HealthCheckType:        "none",
		AllowNodeMigration:     opts.AllowMigration,
		HealthCheckAutoRestart: opts.AllowMigration,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, code, err := client.CreateTemplate(ctx, req)
	if err != nil {
		return "", fmt.Errorf("create template %s: %w (status %d)\njob:\n%s", name, err, code, req.Job)
	}
	return id, nil
}
