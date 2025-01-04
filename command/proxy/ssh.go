package command_proxy

import (
	"strconv"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/proxy"

	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <service> <port> [flags]",
	Short: "Forward a SSH connection via the proxy server",
	Long: `Forwards a SSH connection to a remote SSH server via the proxy server.

If <port> is not given then the port is found via a DNS SRV lookup against the service name.

  service   The name of the remote service to connect to e.g. ssh.service.consul
  port      The optional remote port to connect to e.g. 22`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		var port int
		var err error

		cfg := config.GetServerAddr()

		if len(args) == 2 {
			port, err = strconv.Atoi(args[1])
			if err != nil || port < 1 || port > 65535 {
				cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
			}
		} else {
			port = 0
		}

		proxy.RunSSHForwarderViaProxy(cfg.WsServer, cfg.ApiToken, args[0], port)
	},
}
