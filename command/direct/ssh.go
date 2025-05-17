package commands_direct

import (
	"io"
	"net"
	"os"
	"strconv"

	"github.com/paularlott/knot/internal/util"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var sshCmd = &cobra.Command{
	Use:   "ssh <service> <port> [flags]",
	Short: "Forward a SSH connection to the service",
	Long: `Forwards a SSH connection to a remote SSH server via a direct connection.

If <port> is not given then the port is found via a DNS SRV lookup against the service name.

  service   The name of the remote service to connect to e.g. ssh.service.consul
  port      The optional remote port to connect to e.g. 22`,
	Args: cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		var host string
		var port string
		var err error

		service := args[0]

		if len(args) == 2 {
			var portInt int

			portInt, err = strconv.Atoi(args[1])
			port = strconv.Itoa(portInt)
			if err != nil || portInt < 1 || portInt > 65535 {
				cobra.CheckErr("Invalid port number, port numbers must be between 1 and 65535")
			}

			ips, err := util.LookupIP(service)
			if err != nil {
				cobra.CheckErr("Failed to find service")
			}

			host = (*ips)[0]
		} else {
			hostPorts, err := util.LookupSRV(service)
			if err != nil {
				cobra.CheckErr("Failed to find service")
			}

			host = (*hostPorts)[0].Host
			port = (*hostPorts)[0].Port
		}

		log.Info().Msgf("ssh: forwarding to %s (%s:%s)", args[0], host, port)

		for {
			remoteConn, err := net.Dial("tcp", net.JoinHostPort(host, port))
			if err != nil {
				log.Fatal().Msg("ssh: can't connect to remote")
			}

			go func() { io.Copy(os.Stdout, remoteConn) }()
			_, err = io.Copy(remoteConn, os.Stdin)
			if err != nil {
				remoteConn.Close()
				log.Fatal().Msg("ssh: lost connection to remote")
			}
		}
	},
}
