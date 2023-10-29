package commands_forward

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
  Use:   "ssh <box> [flags]",
  Short: "Forward a SSH connection via the agent",
  Long:  `Forwards a SSH connection to a container running the agent via the proxy server.

  box   The name of the box to connect to e.g. mybox`,
  Args: cobra.ExactArgs(1),
  Run: func(cmd *cobra.Command, args []string) {
    cfg := command.GetServerAddr()
    proxy.RunSSHForwarderViaAgent(cfg.WsServer, args[0])
  },
}
