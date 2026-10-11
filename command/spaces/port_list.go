package command_spaces

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var PortListCmd = &cli.Command{
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
		spaceName := cmd.GetStringArg("space")

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

		// Get the list of port forwards
		response, code, err := client.ListPorts(ctx, spaceId)
		if err != nil {
			return spaceApiError(code, err, "list port forwards", spaceName)
		}

		if len(response.Forwards) == 0 {
			fmt.Printf("No active port forwards in space '%s'.\n", spaceName)
			return nil
		}

		// Build a lookup map for resolving space names
		spaceNames := make(map[string]string, len(spaces.Spaces))
		for _, s := range spaces.Spaces {
			spaceNames[s.Id] = s.Name
		}

		fmt.Printf("Active port forwards in space '%s':\n", spaceName)
		for _, fwd := range response.Forwards {
			persist := "temporary"
			if fwd.Persistent {
				persist = "persistent"
			}
			mode := fwd.Mode
			if mode == "" {
				mode = "—"
			}
			// The server returns space names for display, but fall back to UUID lookup
			target := fwd.Space
			if name, ok := spaceNames[fwd.Space]; ok {
				target = name
			}
			line := fmt.Sprintf("  %d -> %s:%d (%s, %s", fwd.LocalPort, target, fwd.RemotePort, persist, mode)

			// Throttle info
			var throttle []string
			if fwd.LatencyMs > 0 {
				t := fmt.Sprintf("%dms", fwd.LatencyMs)
				if fwd.JitterMs > 0 {
					t += fmt.Sprintf(" ±%dms", fwd.JitterMs)
				}
				throttle = append(throttle, t)
			}
			if fwd.BandwidthKB > 0 {
				throttle = append(throttle, fmt.Sprintf("%dKB/s", fwd.BandwidthKB))
			}
			if fwd.Down {
				throttle = append(throttle, "down")
			}
			if fwd.TimeoutMs > 0 {
				throttle = append(throttle, fmt.Sprintf("timeout=%dms", fwd.TimeoutMs))
			}
			if len(throttle) > 0 {
				line += ", " + strings.Join(throttle, " ")
			}

			line += ")"
			fmt.Println(line)
		}

		return nil
	},
}
