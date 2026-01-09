package command_port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command"

	"github.com/paularlott/cli"
)

var ForwardCmd = &cli.Command{
	Name:        "forward",
	Usage:       "Forward a port from one space to another",
	Description: "Forward a port from one space to a port in another space.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "from-space",
			Usage:    "The name of the source space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "from-port",
			Usage:    "The port in the source space to forward from",
			Required: true,
		},
		&cli.StringArg{
			Name:     "to-space",
			Usage:    "The name of the target space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "to-port",
			Usage:    "The port in the target space to forward to",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		fromSpace := cmd.GetStringArg("from-space")
		fromPort := cmd.GetIntArg("from-port")
		toSpace := cmd.GetStringArg("to-space")
		toPort := cmd.GetIntArg("to-port")

		// Validate port ranges
		if fromPort < 1 || fromPort > 65535 {
			return fmt.Errorf("invalid from-port: must be between 1 and 65535")
		}
		if toPort < 1 || toPort > 65535 {
			return fmt.Errorf("invalid to-port: must be between 1 and 65535")
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
			if s.Name == fromSpace {
				spaceId = s.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("space '%s' not found", fromSpace)
		}

		// Create the request
		request := &apiclient.PortForwardRequest{
			LocalPort:  uint16(fromPort),
			Space:      toSpace,
			RemotePort: uint16(toPort),
		}

		// Send the port forward request
		code, err := client.ForwardPort(ctx, spaceId, request)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to forward ports")
			} else if code == 404 {
				return fmt.Errorf("space not found or not running")
			}
			return fmt.Errorf("port forward failed: %w", err)
		}

		fmt.Printf("Port forward established: %s:%d -> %s:%d\n", fromSpace, fromPort, toSpace, toPort)
		return nil
	},
}
