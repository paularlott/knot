// Package harness boots real knot servers and spaces for black-box API
// integration tests. Tests live in ../suites behind the `integration` build
// tag; this package itself is always compiled so it stays type-checked.
package harness

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Config holds the environment driven test configuration, loaded from the
// environment (and the repo root .env via env.Load in TestMain).
type Config struct {
	// Runtime is the local container runtime: "docker" or "apple".
	Runtime string
	// DockerCache is the registry cache prefix for image pulls, e.g.
	// "hub.anaconda.ovh/docker". Empty means pull from Docker Hub directly.
	DockerCache string
	// ImageNamespace is the registry namespace holding the knot base images.
	ImageNamespace string
	// Image is the base image (name:tag) used for test spaces.
	Image string
	// ContainerHost overrides the host address handed to spaces (agent
	// endpoint / server URL). Empty means the server resolves its own IP
	// via the ${{ host_ip }} token — its primary physical interface.
	ContainerHost string
	// Zone is the knot zone name for test servers.
	Zone string
	// Keep leaves servers, data dirs and containers running after the run
	// for post-failure inspection.
	Keep bool
	// VerboseServer streams server logs to the test output.
	VerboseServer bool
	// NoBuild skips rebuilding the server and agent binaries.
	NoBuild bool
	// SpaceReadyTimeout bounds waiting for a space to boot its agent.
	SpaceReadyTimeoutSeconds int

	// DockerHost is the resolved docker endpoint for the daemon in use
	// (DOCKER_HOST env or the active docker context). Passed to the server
	// as --docker-host because the default /var/run/docker.sock often
	// doesn't exist (colima/lima/etc).
	DockerHost string
}

// ImageRef returns the fully qualified image reference used for spaces,
// e.g. "hub.anaconda.ovh/docker/paularlott/knot-ubuntu:26.04".
func (c *Config) ImageRef() string {
	return c.Registry() + "/" + c.Image
}

// Registry returns the base image registry including namespace, honouring
// the cache prefix, e.g. "hub.anaconda.ovh/docker/paularlott". This value is
// passed to the server as --base-image-registry so templates referencing
// ${{ .server.base_image_registry }} resolve through the cache.
func (c *Config) Registry() string {
	parts := []string{}
	if c.DockerCache != "" {
		parts = append(parts, strings.TrimSuffix(c.DockerCache, "/"))
	}
	if c.ImageNamespace != "" {
		parts = append(parts, strings.TrimSuffix(c.ImageNamespace, "/"))
	}
	return strings.Join(parts, "/")
}

// CacheImageRef maps a docker.io reference through the cache prefix, for
// pulling third-party helper images (vault, victoria-logs, ...).
func (c *Config) CacheImageRef(ref string) string {
	if c.DockerCache == "" {
		return ref
	}
	return strings.TrimSuffix(c.DockerCache, "/") + "/" + ref
}

// LoadConfig builds the configuration from KNOT_TEST_* environment variables
// with defaults suitable for a Docker Desktop dev machine.
func LoadConfig() *Config {
	cfg := &Config{
		Runtime:                  envOr("KNOT_TEST_RUNTIME", "docker"),
		DockerCache:              envOr("KNOT_TEST_DOCKER_CACHE", ""),
		ImageNamespace:           envOr("KNOT_TEST_IMAGE_NAMESPACE", "paularlott"),
		Image:                    envOr("KNOT_TEST_IMAGE", "knot-ubuntu:26.04"),
		// ContainerHost overrides the address handed to spaces for the
		// agent endpoint / server URL. Empty means knot's own ${{
		// host_ip }} resolution: the server picks its primary physical
		// interface IP at startup, so no per-platform hostname (e.g.
		// host.docker.internal) is assumed.
		ContainerHost:            envOr("KNOT_TEST_CONTAINER_HOST", ""),
		Zone:                     envOr("KNOT_TEST_ZONE", "core"),
		Keep:                     envBool("KNOT_TEST_KEEP", false),
		VerboseServer:            envBool("KNOT_TEST_VERBOSE_SERVER", false),
		NoBuild:                  envBool("KNOT_TEST_NO_BUILD", false),
		SpaceReadyTimeoutSeconds: envInt("KNOT_TEST_SPACE_READY_TIMEOUT", 600),
	}

	if cfg.Runtime == "docker" {
		cfg.DockerHost = resolveDockerHost()
	}

	return cfg
}

// resolveDockerHost finds the daemon endpoint the docker CLI itself uses.
func resolveDockerHost() string {
	if v := os.Getenv("DOCKER_HOST"); v != "" {
		return v
	}
	out, err := exec.Command("docker", "context", "inspect",
		"--format", "{{.Endpoints.docker.Host}}").Output()
	if err == nil {
		if host := strings.TrimSpace(string(out)); host != "" {
			return host
		}
	}
	return "unix:///var/run/docker.sock"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
