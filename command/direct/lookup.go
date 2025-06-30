package commands_direct

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var LookupCmd = &cli.Command{
	Name:        "lookup",
	Usage:       "Lookup service",
	Description: "Looks up the IP & port of a service via a DNS SRV lookup against the service name.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "service",
			Usage:    "The name of the service to look up",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		service := cmd.GetStringArg("service")

		hostPorts, err := util.LookupSRV(service)
		if err != nil {
			return fmt.Errorf("Failed to find service: %w", err)
		}

		fmt.Println("\nservice: ", service)
		for _, hp := range hostPorts {
			fmt.Println("  ", hp.Host, hp.Port)
		}
		return nil
	},
}
