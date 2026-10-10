package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var PortStopCmd = &cli.Command{
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
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		spaces, _, err := client.GetSpaces(ctx, "", false)
		if err != nil {
			return fmt.Errorf("couldn't list spaces: %w", err)
		}

		var spaceId string
		for _, s := range spaces.Spaces {
			if s.Name == spaceName {
				spaceId = s.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("space %q not found", spaceName)
		}

		// Create the request
		request := &apiclient.PortStopRequest{
			LocalPort: uint16(localPort),
		}

		// Send the port stop request
		code, err := client.StopPort(ctx, spaceId, request)
		if err != nil {
			return spaceApiError(code, err, "stop the port forward", spaceName)
		}

		fmt.Printf("Port forward on port %d stopped in space '%s'.\n", localPort, spaceName)
		return nil
	},
}
