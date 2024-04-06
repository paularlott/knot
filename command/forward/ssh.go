package commands_forward

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <space> [flags]",
	Short: "Forward a SSH connection via the agent",
	Long: `Forwards a SSH connection to a container running the agent via the proxy server.

  space   The name of the space to connect to e.g. test1`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := command.GetServerAddr()

		proxy.RunSSHForwarderViaAgent(cfg.WsServer, args[0], cfg.ApiToken)
	},
}
