package scaffold

import (
	_ "embed"
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
