package commands_forward

import (
	"strconv"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/proxy"
	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
)

var portCmd = &cobra.Command{
	Use:   "port <listen> <space> <port> [flags]",
	Short: "Forward a port via the agent",
	Long: `Forwards a local port to a remote container running the agent via the proxy server.

  listen    The local port to listen on e.g. :8080
  space     The name of the space to connect to e.g. test1
  port      The remote port to connect to e.g. 80`,
	Args: cobra.ExactArgs(3),
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)

		port, err := strconv.Atoi(args[2])
		if err != nil || port < 1 || port > 65535 {
			cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
		}

		proxy.RunTCPForwarderViaAgent(cfg.WsServer, util.FixListenAddress(args[0]), args[1], port, cfg.ApiToken)
	},
}
