package mcp

import (
	"context"
	_ "embed"
	"fmt"

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

	switch platform {
	case "docker", "podman":
		return mcp.NewToolResponseText(dockerPodmanSpec), nil
	case "nomad":
		return mcp.NewToolResponseText(nomadSpec), nil
	default:
		return nil, fmt.Errorf("platform must be one of: docker, podman, nomad")
	}
}
