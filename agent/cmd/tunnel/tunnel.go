package command_tunnel

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/cli"
	"github.com/rs/zerolog/log"
)

var TunnelCmd = &cli.Command{
	Name:  "tunnel",
	Usage: "Open a tunnel",
	Description: `The tunnel command allows you to create a tunnel to expose a local port on the internet.

The tunnel can be created to expose either an http or https endpoint, the name provided is prepended with the username e.g. <user>--<tunnel_name>.<domain>.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "protocol",
			Usage:    "The protocol to use (http or https)",
			Required: true,
		},
		&cli.IntArg{
			Name:     "port",
			Usage:    "The local port to tunnel to",
			Required: true,
		},
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name to expose the tunnel as",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server to create the tunnel on.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The token to use for authentication.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:    "port-tls-name",
			Usage:   "The name to present to local port when using.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TLS_NAME"},
		},
		&cli.BoolFlag{
			Name:         "port-tls-skip-verify",
			Usage:        "Skip TLS verification when talking to local port via https, this allows self signed certificates.",
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_PORT_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		// Validate the protocol
		if cmd.GetStringArg("protocol") != "http" && cmd.GetStringArg("protocol") != "https" {
			return fmt.Errorf("Invalid protocol type, must be either http or https")
		}

		// Validate that the port is a number
		port := cmd.GetIntArg("port")
		if port < 1 || port > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535")
		}

		// Validate the name is all lowercase and only contains letters, numbers and dashes
		if !validate.Name(cmd.GetStringArg("name")) {
			return fmt.Errorf("Invalid name, must be all lowercase and only contain letters, numbers and dashes")
		}

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client := tunnel_server.NewTunnelClient(
			cfg.WsServer,
			cfg.HttpServer,
			cfg.ApiToken,
			cmd.GetBool("tls-skip-verify"),
			&tunnel_server.TunnelOpts{
				Type:          tunnel_server.WebTunnel,
				Protocol:      cmd.GetStringArg("protocol"),
				LocalPort:     uint16(port),
				TunnelName:    cmd.GetStringArg("name"),
				TlsName:       cmd.GetString("port-tls-name"),
				TlsSkipVerify: cmd.GetBool("port-tls-skip-verify"),
			},
		)
		if err := client.ConnectAndServe(); err != nil {
			log.Fatal().Err(err).Msg("Failed to create tunnel")
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
