package commands_forward

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
  Use:   "ssh <space> [flags]",
  Short: "Forward a SSH connection via the agent",
  Long:  `Forwards a SSH connection to a container running the agent via the proxy server.

  space   The ID of the space to connect to e.g. a08ffda8-8b57-4047-9370-d032819d2c8f`,
  Args: cobra.ExactArgs(1),
  Run: func(cmd *cobra.Command, args []string) {
    cfg := command.GetServerAddr()
    proxy.RunSSHForwarderViaAgent(cfg.WsServer, args[0], cfg.ApiToken)
  },
}
