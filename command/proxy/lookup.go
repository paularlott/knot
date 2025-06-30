package command_proxy

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/api"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/cli"
)

var LookupCmd = &cli.Command{
	Name:  "lookup",
	Usage: "Lookup a service",
	Description: `Looks up the IP & port of a service via a DNS SRV lookup against the service name.

The request is passed to the proxy server to be processed rather than run against the local resolver.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "service",
			Usage:    "The service to look up",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		service := cmd.GetStringArg("service")
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := rest.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		lookup, _, err := api.CallLookup(client, service)
		if err != nil || !lookup.Status {
			fmt.Println("Failed to parse response")
			os.Exit(1)
		}

		fmt.Println("\nservice: ", service)
		fmt.Println("   host: ", lookup.Host)
		fmt.Println("   port: ", lookup.Port)
		return nil
	},
}
