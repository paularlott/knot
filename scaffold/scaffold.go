package scaffold

import (
	_ "embed"
)

var (
  //go:embed client.yml
  ClientScaffold string

  //go:embed server.yml
  ServerScaffold string

  //go:embed agent.yml
  AgentScaffold string
)
