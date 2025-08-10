package mcp

import (
	"context"
	_ "embed"
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/mcp"
)

var (
	//go:embed specs/docker-podman-spec.md
	dockerPodmanSpec string

	//go:embed specs/nomad-spec.md
	nomadSpec string
)

func getPlatformSpec(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	platform, err := req.String("platform")
	if err != nil {
		return nil, fmt.Errorf("platform parameter is required")
	}

	var spec string
	switch platform {
	case "docker":
		spec, err = getDockerSpec()
	case "podman":
		spec, err = getPodmanSpec()
	case "nomad":
		spec, err = getNomadSpec()
	default:
		return nil, fmt.Errorf("platform must be one of: docker, podman, nomad")
	}

	if err != nil {
		return nil, err
	}

	return mcp.NewToolResponseText(spec), nil
}

func getNomadSpec() (string, error) {
	cfg := config.GetServerConfig()
	if cfg.Chat.NomadSpecFile != "" {
		content, err := os.ReadFile(cfg.Chat.NomadSpecFile)
		if err != nil {
			return "", fmt.Errorf("failed to read nomad spec file %s: %w", cfg.Chat.NomadSpecFile, err)
		}
		return string(content), nil
	}
	return nomadSpec, nil
}

func getDockerSpec() (string, error) {
	cfg := config.GetServerConfig()
	if cfg.Chat.DockerSpecFile != "" {
		content, err := os.ReadFile(cfg.Chat.DockerSpecFile)
		if err != nil {
			return "", fmt.Errorf("failed to read docker spec file %s: %w", cfg.Chat.DockerSpecFile, err)
		}
		return string(content), nil
	}
	return dockerPodmanSpec, nil
}

func getPodmanSpec() (string, error) {
	cfg := config.GetServerConfig()
	if cfg.Chat.PodmanSpecFile != "" {
		content, err := os.ReadFile(cfg.Chat.PodmanSpecFile)
		if err != nil {
			return "", fmt.Errorf("failed to read podman spec file %s: %w", cfg.Chat.PodmanSpecFile, err)
		}
		return string(content), nil
	}
	return dockerPodmanSpec, nil
}

// GetInternalNomadSpec returns the embedded nomad spec (for scaffold command)
func GetInternalNomadSpec() string {
	return nomadSpec
}

// GetInternalDockerPodmanSpec returns the embedded docker/podman spec (for scaffold command)
func GetInternalDockerPodmanSpec() string {
	return dockerPodmanSpec
}
