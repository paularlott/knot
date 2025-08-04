package mcp

import (
	"context"
	_ "embed"

	"github.com/paularlott/mcp"
)

//go:embed specs/docker-podman-spec.md
var dockerPodmanSpec string

func getContainerSpec(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	return mcp.NewToolResponseText(dockerPodmanSpec), nil
}
