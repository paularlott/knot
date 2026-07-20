package command_spaces

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"

	"github.com/paularlott/cli"
)

var PortThrottleCmd = &cli.Command{
	Name:        "throttle",
	Usage:       "Add latency, jitter, and/or bandwidth limits to a port forward",
	Description: "Apply network simulation to an existing port forward in a space. All values are optional; pass --reset to clear all limits.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "local-port",
			Usage:    "The local port of the forward to throttle",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "latency",
			Usage: "Latency in milliseconds (e.g. 50, 200ms)",
		},
		&cli.StringFlag{
			Name:  "jitter",
			Usage: "Jitter in milliseconds (e.g. 10, 50ms)",
		},
		&cli.StringFlag{
			Name:  "bandwidth",
			Usage: "Bandwidth limit in KB/s (e.g. 100, 1024)",
		},
		&cli.StringFlag{
			Name:  "timeout",
			Usage: "Connection timeout in milliseconds (e.g. 5000) — kills the connection after this duration",
		},
		&cli.BoolFlag{
			Name:  "down",
			Usage: "Block all traffic on this forward (port definition stays)",
		},
		&cli.BoolFlag{
			Name:  "reset",
			Usage: "Clear all throttle settings",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		localPort := cmd.GetIntArg("local-port")
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("invalid local port, must be between 1 and 65535")
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		spaces, _, err := client.GetSpaces(ctx, "", false)
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

		request := apiclient.PortThrottleRequest{
			LocalPort: uint16(localPort),
			Reset:     cmd.GetBool("reset"),
		}

		if !request.Reset {
			if v := cmd.GetString("latency"); v != "" {
				ms, err := parseMsVal(v)
				if err != nil {
					return fmt.Errorf("invalid latency: %w", err)
				}
				request.LatencyMs = ms
			}
			if v := cmd.GetString("jitter"); v != "" {
				ms, err := parseMsVal(v)
				if err != nil {
					return fmt.Errorf("invalid jitter: %w", err)
				}
				request.JitterMs = ms
			}
			if v := cmd.GetString("bandwidth"); v != "" {
				kb, err := strconv.Atoi(v)
				if err != nil || kb <= 0 {
					return fmt.Errorf("invalid bandwidth, must be a positive number in KB/s")
				}
				request.BandwidthKB = kb
			}
			if v := cmd.GetString("timeout"); v != "" {
				ms, err := parseMsVal(v)
				if err != nil {
					return fmt.Errorf("invalid timeout: %w", err)
				}
				request.TimeoutMs = ms
			}
			request.Down = cmd.GetBool("down")
		}

		code, err := client.ThrottlePort(ctx, spaceId, &request)
		if err != nil {
			if code == 401 {
				return fmt.Errorf("failed to authenticate with server, check token")
			} else if code == 403 {
				return fmt.Errorf("no permission to throttle port forwards")
			} else if code == 404 {
				return fmt.Errorf("space not found")
			} else if code == 409 {
				return fmt.Errorf("space is not running")
			}
			return fmt.Errorf("failed to set throttle: %w", err)
		}

		if request.Reset {
			fmt.Printf("Throttle cleared for port %d in space '%s'\n", localPort, spaceName)
		} else {
			parts := []string{}
			if request.LatencyMs > 0 {
				parts = append(parts, fmt.Sprintf("latency=%dms", request.LatencyMs))
			}
			if request.JitterMs > 0 {
				parts = append(parts, fmt.Sprintf("jitter=%dms", request.JitterMs))
			}
			if request.BandwidthKB > 0 {
				parts = append(parts, fmt.Sprintf("bandwidth=%dKB/s", request.BandwidthKB))
			}
			if len(parts) == 0 {
				parts = []string{"no limits set"}
			}
			fmt.Printf("Throttle set for port %d in space '%s': %s\n", localPort, spaceName, strings.Join(parts, ", "))
		}
		return nil
	},
}

func parseMsVal(s string) (int, error) {
	s = strings.TrimSuffix(strings.TrimSpace(s), "ms")
	return strconv.Atoi(s)
}
