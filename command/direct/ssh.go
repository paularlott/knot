package commands_direct

import (
	"context"
	"io"
	"net"
	"os"
	"strconv"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
	"github.com/rs/zerolog/log"
)

var SshCmd = &cli.Command{
	Name:  "ssh",
	Usage: "Forward SSH to a service",
	Description: `Forwards a SSH connection to a remote SSH server via a direct connection.

If [port] is not given then the port is found via a DNS SRV lookup against the service name.`,
	Arguments: []cli.Argument{
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

		service := cmd.GetStringArg("service")

		if cmd.HasArg("port") {
			var portInt int
			portInt, err = strconv.Atoi(cmd.GetStringArg("port"))
			port = strconv.Itoa(portInt)
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

		log.Info().Msgf("ssh: forwarding to %s (%s:%s)", service, host, port)

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
