package command_port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command"

	"github.com/paularlott/cli"
)

var StopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a port forward",
	Description: "Stop an active port forward by local port number.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "local-port",
			Usage:    "The local port to stop forwarding",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		localPort := cmd.GetIntArg("local-port")

		// Validate port range
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("invalid local-port: must be between 1 and 65535")
		}

		// Get the space ID from the space name
		client, err := command.GetClient(cmd)
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

		// Create the request
		request := &apiclient.PortStopRequest{
			LocalPort: uint16(localPort),
		}

		// Send the port stop request
		code, err := client.StopPort(ctx, spaceId, request)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to stop port forwards")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			}
			return fmt.Errorf("failed to stop port forward: %w", err)
		}

		fmt.Printf("Port forward on port %d stopped in space '%s'.\n", localPort, spaceName)
		return nil
	},
}
