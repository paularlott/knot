package command

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var PingCmd = &cli.Command{
	Name:        "ping",
	Usage:       "Ping the server",
	Description: "Ping the server and display the health and version number.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server to ping.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The token to use for authentication.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")

		// Get the server address from config using alias
		cfg := config.GetServerAddr(alias, cmd)
		fmt.Println("Pinging server: ", cfg.HttpServer)

		client, err := apiclient.NewClient(
			cfg.HttpServer,
			cfg.ApiToken,
			cmd.GetBool("tls-skip-verify"),
		)
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		version, err := client.Ping(ctx)
		if err != nil {
			return fmt.Errorf("Failed to ping server: %w", err)
		}

		fmt.Println("\nServer is healthy")
		fmt.Println("Version: ", version.Version)
		fmt.Println("Zone: ", version.Zone)

		return nil
	},
}
