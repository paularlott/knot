package command_spaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var PortForwardCmd = &cli.Command{
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
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "persistent",
			Aliases: []string{"p"},
			Usage:   "Persist the port forward across agent restarts",
		},
		&cli.BoolFlag{
			Name:    "force",
			Aliases: []string{"f"},
			Usage:   "Create the forward even if the target space is not currently running",
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
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		spaces, _, err := client.GetSpaces(ctx, "", false)
		if err != nil {
			return fmt.Errorf("couldn't list spaces: %w", err)
		}

		var fromSpaceInfo *apiclient.SpaceInfo
		for i := range spaces.Spaces {
			if spaces.Spaces[i].Name == fromSpace {
				fromSpaceInfo = &spaces.Spaces[i]
				break
			}
		}

		if fromSpaceInfo == nil {
			return fmt.Errorf("space %q not found", fromSpace)
		}

		if !fromSpaceInfo.IsDeployed || !fromSpaceInfo.HasState {
			if !cmd.GetBool("persistent") {
				return fmt.Errorf("space %q is not running", fromSpace)
			}
		}

		// The target may be an own space (bare name), an own pool, or
		// another user's space or pool as user--name (ports the target
		// template marks public). Only own spaces can be running-checked
		// here; the server resolves and authorizes the rest.
		if !cmd.GetBool("force") && !strings.Contains(toSpace, "--") {
			var toSpaceInfo *apiclient.SpaceInfo
			for i := range spaces.Spaces {
				if spaces.Spaces[i].Name == toSpace {
					toSpaceInfo = &spaces.Spaces[i]
					break
				}
			}
			if toSpaceInfo != nil && !toSpaceInfo.IsDeployed && !toSpaceInfo.HasState {
				return fmt.Errorf("space %q is not running", toSpace)
			}
		}

		spaceId := fromSpaceInfo.Id

		force := cmd.GetBool("force")

		// Send the target reference; the server resolves it (space, pool,
		// user--name) and checks access.
		request := &apiclient.PortForwardRequest{
			LocalPort:  uint16(fromPort),
			Space:      toSpace,
			RemotePort: uint16(toPort),
			Persistent: cmd.GetBool("persistent"),
			Force:      force,
		}

		// Send the port forward request
		_, err = client.ForwardPort(ctx, spaceId, request)
		if err != nil {
			// The server's reason (target not found, port not public, pool
			// empty, target not running) is carried by the wrapped error.
			return fmt.Errorf("couldn't forward %s:%d to %s:%d: %w", fromSpace, fromPort, toSpace, toPort, err)
		}

		fmt.Printf("Port forward established: %s:%d -> %s:%d\n", fromSpace, fromPort, toSpace, toPort)
		return nil
	},
}
