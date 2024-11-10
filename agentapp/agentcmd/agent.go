package agentcmd

import (
	"github.com/paularlott/knot/agentapp/agentcmd/agentcmd"
)

func init() {
	RootCmd.AddCommand(agentcmd.AgentCmd)
}
