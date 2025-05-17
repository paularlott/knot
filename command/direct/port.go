package commands_direct

import (
	"io"
	"net"
	"strconv"

	"github.com/paularlott/knot/internal/util"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var portCmd = &cobra.Command{
	Use:   "port <listen> <service> <port> [flags]",
	Short: "Forward a local port to the service",
	Long: `Forwards a local port to a remote server and port via a direct connection.

If <port> is not given then the remote port is found via a DNS SRV lookup against the service name.

  listen    The local port to listen on e.g. :8080
  service   The name of the remote service to connect to e.g. web.service.consul
  port      The optional remote port to connect to e.g. 80`,
	Args: cobra.RangeArgs(2, 3),
	Run: func(cmd *cobra.Command, args []string) {
		var host string
		var port string
		var err error

		listen := util.FixListenAddress(args[0])
		service := args[1]

		if len(args) == 3 {
			var portInt int

			portInt, err = strconv.Atoi(args[2])
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

		log.Info().Msgf("port: listening on %s", listen)
		log.Info().Msgf("port: forwarding to %s (%s:%s)", args[0], host, port)

		listener, err := net.Listen("tcp", listen)
		if err != nil {
			log.Fatal().Msgf("port: error while opening local port: %s", err.Error())
		}
		defer listener.Close()

		for {
			localConn, err := listener.Accept()
			if err != nil {
				log.Error().Msgf("port: could not accept the connection: %s", err.Error())
			}

			go func() {
				remoteConn, err := net.Dial("tcp", net.JoinHostPort(host, port))
				if err != nil {
					localConn.Close()
					log.Fatal().Msg("port: can't connect to remote")
				}
				defer remoteConn.Close()

				go func() { io.Copy(localConn, remoteConn) }()
				_, err = io.Copy(remoteConn, localConn)
				if err != nil {
					localConn.Close()
					remoteConn.Close()
					log.Fatal().Msg("port: lost connection to remote")
				}
			}()
		}
	},
}
