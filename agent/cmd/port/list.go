package port

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var ListPortForwardsCmd = &cli.Command{
	Name:        "list",
	Usage:       "List active port forwards",
	Description: "List all active port forwards in the current space.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var response agentlink.ListPortForwardsResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandListPortForwards, nil, &response)
		if err != nil {
			return fmt.Errorf("error listing port forwards: %w", err)
		}

		if len(response.Forwards) == 0 {
			fmt.Println("No active port forwards.")
			return nil
		}

		fmt.Println("Active port forwards:")
		for _, fwd := range response.Forwards {
			line := fmt.Sprintf("  %d -> %s:%d", fwd.LocalPort, fwd.Space, fwd.RemotePort)
			if fwd.Persistent {
				line += " (persistent"
			} else {
				line += " (temporary"
			}
			mode := fwd.Mode
			if mode == "" {
				mode = "relay"
			}
			line += ", " + mode

			// Throttle info
			var throttle []string
			if fwd.LatencyMs > 0 {
				throttle = append(throttle, fmt.Sprintf("%dms", fwd.LatencyMs))
				if fwd.JitterMs > 0 {
					throttle = append(throttle, fmt.Sprintf("±%dms", fwd.JitterMs))
				}
			}
			if fwd.BandwidthKB > 0 {
				throttle = append(throttle, fmt.Sprintf("%dKB/s", fwd.BandwidthKB))
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
