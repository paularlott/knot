package commands_direct

import (
	"context"
	"io"
	"net"
	"strconv"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
	"github.com/rs/zerolog/log"
)

var PortCmd = &cli.Command{
	Name:  "port",
	Usage: "Forward port to service",
	Description: `Forwards a local port to a remote server and port via a direct connection.

If [port] is not given then the remote port is found via a DNS SRV lookup against the service name.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "listen",
			Usage:    "The local port to listen on",
			Required: true,
		},
		&cli.StringArg{
			Name:     "service",
			Usage:    "The name of the remote service to connect to",
			Required: true,
		},
		&cli.StringArg{
			Name:     "port",
			Usage:    "The remote port to connect to",
			Required: false,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var host string
		var port string
		var err error

		listen := util.FixListenAddress(cmd.GetStringArg("listen"))
		service := cmd.GetStringArg("service")
		port = cmd.GetStringArg("port")

		if cmd.HasArg("port") {
			var portInt int
			portInt, err = strconv.Atoi(port)
			if err != nil || portInt < 1 || portInt > 65535 {
				log.Fatal().Msg("Invalid port number, port numbers must be between 1 and 65535")
			}

			ips, err := util.LookupIP(service)
			if err != nil || len(ips) == 0 {
				log.Fatal().Msg("Failed to find service")
			}
			host = ips[0]
		} else {
			hostPorts, err := util.LookupSRV(service)
			if err != nil || len(hostPorts) == 0 {
				log.Fatal().Msg("Failed to find service")
			}

			host = hostPorts[0].Host
			port = hostPorts[0].Port
		}

		log.Info().Msgf("port: listening on %s", listen)
		log.Info().Msgf("port: forwarding to %s (%s:%d)", service, host, port)

		listener, err := net.Listen("tcp", listen)
		if err != nil {
			log.Fatal().Msgf("port: error while opening local port: %s", err.Error())
		}
		defer listener.Close()

		for {
			localConn, err := listener.Accept()
			if err != nil {
				log.Error().Msgf("port: could not accept the connection: %s", err.Error())
				continue
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
