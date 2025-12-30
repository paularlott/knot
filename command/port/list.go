package command_port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List active port forwards for a space",
	Description: "List all active port forwards from a space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		spaceName := cmd.GetStringArg("space")

		// Get the space ID from the space name
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		spaces, _, err := client.GetSpaces(ctx, "")
		if err != nil {
			return fmt.Errorf("failed to get spaces: %w", err)
		}

		var spaceId string
		for _, s := range spaces.Spaces {
			if s.Name == spaceName {
				spaceId = s.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("space '%s' not found", spaceName)
		}

		// Get the list of port forwards
		response, code, err := client.ListPorts(ctx, spaceId)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to list port forwards")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			}
			return fmt.Errorf("failed to list port forwards: %w", err)
		}

		if len(response.Forwards) == 0 {
			fmt.Printf("No active port forwards in space '%s'.\n", spaceName)
			return nil
		}

		fmt.Printf("Active port forwards in space '%s':\n", spaceName)
		for _, fwd := range response.Forwards {
			fmt.Printf("  %d -> %s:%d\n", fwd.LocalPort, fwd.Space, fwd.RemotePort)
		}

		return nil
	},
}
