package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/cli"
)

// TunnelCmd is the `knot space tunnel` group: remote management of a space's
// agent-owned web tunnels. Tunnels started here are owned by the space's agent
// (daemon is implied) and run until the agent exits or they are stopped.
var TunnelCmd = &cli.Command{
	Name:        "tunnel",
	Usage:       "Manage a space's web tunnels",
	Description: `Start and manage agent-owned web tunnels in a space.

A tunnel exposes a port inside the space on the internet as
<user>--<name>.<domain>. The tunnel is owned by the space's agent and runs until
the agent exits or the tunnel is stopped; it is not persisted.`,
	MaxArgs: cli.NoArgs,
	Commands: []*cli.Command{
		spaceTunnelHttpCmd,
		spaceTunnelHttpsCmd,
		spaceTunnelStopCmd,
		spaceTunnelListCmd,
	},
}

func newSpaceTunnelStartCmd(name, protocol string) *cli.Command {
	return &cli.Command{
		Name:        name,
		Usage:       fmt.Sprintf("Start an %s web tunnel in a space", name),
		Description: fmt.Sprintf(`Start an agent-owned %s web tunnel in a space, exposing <port> as <user>--<name>.<domain>.`, protocol),
		Arguments: []cli.Argument{
			&cli.StringArg{
				Name:     "space",
				Usage:    "The name of the space",
				Required: true,
			},
			&cli.IntArg{
				Name:     "port",
				Usage:    "The port within the space to tunnel",
				Required: true,
			},
			&cli.StringArg{
				Name:     "name",
				Usage:    "The name to expose the tunnel as",
				Required: true,
			},
		},
		MaxArgs: cli.NoArgs,
		Run: func(ctx context.Context, cmd *cli.Command) error {
			return runSpaceTunnelStart(ctx, cmd, protocol)
		},
	}
}

var (
	spaceTunnelHttpCmd  = newSpaceTunnelStartCmd("http", "http")
	spaceTunnelHttpsCmd = newSpaceTunnelStartCmd("https", "https")
)

func runSpaceTunnelStart(ctx context.Context, cmd *cli.Command, protocol string) error {
	spaceName := cmd.GetStringArg("space")

	port := cmd.GetIntArg("port")
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port, must be between 1 and 65535")
	}

	name := cmd.GetStringArg("name")
	if !validate.Name(name) {
		return fmt.Errorf("invalid name, must be all lowercase and only contain letters, numbers and dashes")
	}

	client, err := cmdutil.GetClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}

	spaceId, err := resolveSpaceId(ctx, client, spaceName)
	if err != nil {
		return err
	}

	response, code, err := client.StartSpaceTunnel(ctx, spaceId, &apiclient.SpaceTunnelStartRequest{
		Protocol: protocol,
		Port:     uint16(port),
		Name:     name,
	})
	if err != nil {
		return spaceApiError(code, err, "start tunnel")
	}

	if !response.Success {
		return fmt.Errorf("%s", response.Error)
	}

	fmt.Printf("Tunnel URL: %s\n", response.URL)
	fmt.Println("Tunnel running in agent.")
	return nil
}

var spaceTunnelStopCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a web tunnel in a space",
	Description: `Stop an agent-owned web tunnel in a space by name.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "name",
			Usage:    "The name of the tunnel to stop",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		name := cmd.GetStringArg("name")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		spaceId, err := resolveSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		code, err := client.StopSpaceTunnel(ctx, spaceId, &apiclient.SpaceTunnelStopRequest{Name: name})
		if err != nil {
			return spaceApiError(code, err, "stop tunnel")
		}

		fmt.Printf("Tunnel %s stopped in space '%s'.\n", name, spaceName)
		return nil
	},
}

var spaceTunnelListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List web tunnels in a space",
	Description: `List the agent-owned web tunnels in a space.`,
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

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		spaceId, err := resolveSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		response, code, err := client.ListSpaceTunnels(ctx, spaceId)
		if err != nil {
			return spaceApiError(code, err, "list tunnels")
		}

		if len(response.Tunnels) == 0 {
			fmt.Printf("No active tunnels in space '%s'.\n", spaceName)
			return nil
		}

		fmt.Printf("Active tunnels in space '%s':\n", spaceName)
		for _, t := range response.Tunnels {
			fmt.Printf("  %s  %d  %s  %s\n", t.Name, t.Port, t.Protocol, t.URL)
		}
		return nil
	},
}

// resolveSpaceId resolves a space name (or UUID) to its ID.
func resolveSpaceId(ctx context.Context, client *apiclient.ApiClient, spaceName string) (string, error) {
	if validate.UUID(spaceName) {
		return spaceName, nil
	}

	spaces, _, err := client.GetSpaces(ctx, "", false)
	if err != nil {
		return "", fmt.Errorf("failed to get spaces: %w", err)
	}

	for _, s := range spaces.Spaces {
		if s.Name == spaceName {
			return s.Id, nil
		}
	}

	return "", fmt.Errorf("space '%s' not found", spaceName)
}

// spaceApiError maps common HTTP status codes from the space-io API to
// user-friendly errors.
func spaceApiError(code int, err error, op string) error {
	switch code {
	case 401:
		return fmt.Errorf("failed to authenticate with server, check token")
	case 403:
		return fmt.Errorf("no permission to %s", op)
	case 404:
		return fmt.Errorf("space not found")
	case 409:
		return fmt.Errorf("space is not running")
	default:
		return fmt.Errorf("failed to %s: %w", op, err)
	}
}
