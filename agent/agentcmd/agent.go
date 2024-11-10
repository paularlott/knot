package agentcmd

import (
	"github.com/paularlott/knot/agent/agentcmd/agentcmd"
)

func init() {
	RootCmd.AddCommand(agentcmd.AgentCmd)
}
