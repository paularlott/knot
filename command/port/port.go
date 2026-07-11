package command_port

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/tunnel_server"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/log"
)

// PortCmd is the top-level `knot port` command. It links a service running on
// the local machine into a space so the space can reach it: the space's port is
// forwarded to the local port. The link is active only while the command runs.
var PortCmd = &cli.Command{
	Name:        "port",
	Usage:       "Link a local service into a space",
	Description: `Link a service running on the local machine into a space.

The space's <space-port> is forwarded to the local <local-port>, so processes
inside the space can reach the local service. The link is active only while this
command runs (Ctrl-C to stop).`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to link into",
			Required: true,
		},
		&cli.IntArg{
			Name:     "space-port",
			Usage:    "The port to listen on within the space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "local-port",
			Usage:    "The local port of the service to link",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "server",
			Aliases:  []string{"s"},
			Usage:    "The address of the remote server to manage spaces on.",
			EnvVars:  []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			Global:   true,
		},
		&cli.StringFlag{
			Name:     "token",
			Aliases:  []string{"t"},
			Usage:    "The token to use for authentication.",
			EnvVars:  []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
			Global:   true,
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
			Global:       true,
		},
		&cli.BoolFlag{
			Name:         "tls",
			Usage:        "Enable TLS encryption for the tunnel.",
			ConfigPath:   []string{"tls"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TUNNEL_TLS"},
			DefaultValue: false,
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
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		spaceName := cmd.GetStringArg("space")

		spacePort := cmd.GetIntArg("space-port")
		if spacePort < 1 || spacePort > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535")
		}

		localPort := cmd.GetIntArg("local-port")
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535")
		}

		opts := tunnel_server.TunnelOpts{
			Type:          tunnel_server.PortTunnel,
			Protocol:      "tcp",
			LocalPort:     uint16(localPort),
			SpaceName:     spaceName,
			SpacePort:     uint16(spacePort),
			TlsName:       cmd.GetString("port-tls-name"),
			TlsSkipVerify: cmd.GetBool("port-tls-skip-verify"),
		}

		if cmd.GetBool("tls") {
			opts.Protocol = "tls"
		}

		client := tunnel_server.NewTunnelClient(
			cfg.WsServer,
			cfg.HttpServer,
			cfg.ApiToken,
			cmd.GetBool("tls-skip-verify"),
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
		log.Info("Tunnel shutdown")

		return nil
	},
}
