package commands_forward

import (
	"strconv"

	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"
	"github.com/spf13/cobra"
)

var portCmd = &cobra.Command{
  Use:   "port <listen> <box> <port> [flags]",
  Short: "Forward a port via the agent",
  Long:  `Forwards a local port to a remote container running the agent via the proxy server.

  listen    The local port to listen on e.g. :8080
  box       The name of the box to connect to e.g. mybox
  port      The remote port to connect to e.g. 80`,
  Args: cobra.ExactArgs(2),
  Run: func(cmd *cobra.Command, args []string) {
    var port int
    var err error

    forwardCmdCfg := command.GetProxyFlags()

    port, err = strconv.Atoi(args[1])
    if err != nil || port < 1 || port > 65535 {
      cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
    }

    proxy.RunTCPForwarderViaAgent(forwardCmdCfg.WsServer, args[0], port)
  },
}
