package command_spaces

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"

	"github.com/paularlott/cli"
	"github.com/rs/zerolog/log"
)

var TunnelPortCmd = &cli.Command{
	Name:        "tunnel",
	Usage:       "<space> <listen> <port>",
	Description: `Open a tunnel between a port inside a space and a port on the local machine.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to tunnel from",
			Required: true,
		},
		&cli.IntArg{
			Name:     "listen",
			Usage:    "The port to listen on within the space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "port",
			Usage:    "The local port to connect to",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "tls",
			Usage:        "Enable TLS encryption for the tunnel.",
			ConfigPath:   []string{"tls"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TUNNEL_TLS"},
			DefaultValue: false,
		},
		&cli.StringFlag{
			Name:       "tls-name",
			Usage:      "The name to present to TLS ports.",
			ConfigPath: []string{"tls_name"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_TLS_NAME"},
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		spaceName := cmd.GetStringArg("space")

		listenPort := cmd.GetIntArg("listen")
		if listenPort < 1 || listenPort > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535", 1)
		}

		localPort := cmd.GetIntArg("port")
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535", 1)
		}

		opts := tunnel_server.TunnelOpts{
			Type:          tunnel_server.PortTunnel,
			Protocol:      "tcp",
			LocalPort:     uint16(localPort),
			SpaceName:     spaceName,
			SpacePort:     uint16(listenPort),
			TlsName:       cmd.GetString("tls-name"),
			TlsSkipVerify: true, // FIXME this needs defining see tunnel.go
		}

		if cmd.GetBool("tls") {
			opts.Protocol = "tls"
		}

		client := tunnel_server.NewTunnelClient(
			cfg.WsServer,
			cfg.HttpServer,
			cfg.ApiToken,
			cmd.GetBool("tls-skip-verify"), // FIXME this needs defining see tunnel.go
			&opts,
		)
		if err := client.ConnectAndServe(); err != nil {
			return fmt.Errorf("Failed to create tunnel: %w", err)
		}

		// Wait for ctrl-c
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive ctrl-c or tunnel context is cancelled
		select {
		case <-client.GetCtx().Done():
		case <-c:
		}

		client.Shutdown()

		fmt.Println("\r")
		log.Info().Msg("Tunnel shutdown")

		return nil
	},
}
