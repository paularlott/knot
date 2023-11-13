package command_proxy

import (
	"strconv"

	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/proxy"

	"github.com/spf13/cobra"
)

var portCmd = &cobra.Command{
  Use:   "port <listen> <service> <port> [flags]",
  Short: "Forward a port via the proxy server",
  Long:  `Forwards a local port to a remote server and port via the proxy server.

If <port> is not given then the remote port is found via a DNS SRV lookup against the service name.

  listen    The local port to listen on e.g. :8080
  service   The name of the remote service to connect to e.g. web.service.consul
  port      The optional remote port to connect to e.g. 80`,
  Args: cobra.RangeArgs(2, 3),
  Run: func(cmd *cobra.Command, args []string) {
    var port int
    var err error

    cfg := command.GetServerAddr()

    if len(args) == 3 {
      port, err = strconv.Atoi(args[2])
      if err != nil || port < 1 || port > 65535 {
        cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
      }
    } else {
      port = 0
    }

    proxy.RunTCPForwarderViaProxy(cfg.WsServer, cfg.ApiToken, args[0], args[1], port)
  },
}
