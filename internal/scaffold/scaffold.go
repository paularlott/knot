package scaffold

import (
	_ "embed"

	"github.com/paularlott/knot/internal/systemprompt"
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
	return systemprompt.GetInternalSystemPrompt()
}
