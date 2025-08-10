package scaffold

import (
	_ "embed"

	"github.com/paularlott/knot/internal/chat"
	"github.com/paularlott/knot/internal/mcp"
)

var (
	//go:embed client.toml
	ClientScaffold string

	//go:embed server.toml
	ServerScaffold string

	//go:embed agent.toml
	AgentScaffold string

	//go:embed knot-server.nomad
	NomadScaffold string
)

// GetSystemPromptScaffold returns the embedded system prompt
func GetSystemPromptScaffold() string {
	return chat.GetInternalSystemPrompt()
}

// GetNomadSpecScaffold returns the embedded nomad spec
func GetNomadSpecScaffold() string {
	return mcp.GetInternalNomadSpec()
}

// GetDockerSpecScaffold returns the embedded docker spec
func GetDockerSpecScaffold() string {
	return mcp.GetInternalDockerPodmanSpec()
}

// GetPodmanSpecScaffold returns the embedded podman spec (same as docker)
func GetPodmanSpecScaffold() string {
	return mcp.GetInternalDockerPodmanSpec()
}
