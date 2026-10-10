package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/paularlott/cli"
)

// TunnelCmd is the `knot space tunnel` group: remote management of a space's
// agent-owned web tunnels. Tunnels started here are owned by the space's agent
// (daemon is implied) and run until the agent exits or they are stopped.
var TunnelCmd = &cli.Command{
	Name:  "tunnel",
	Usage: "Manage a space's web tunnels",
	Description: `Start and manage agent-owned web tunnels in a space.

A tunnel exposes a port inside the space on the internet as
<user>--<name>.<domain>. The tunnel is owned by the space's agent and runs until
the agent exits or the tunnel is stopped; it is not persisted.

By default the tunnel is created on the server that owns the space. The
--tunnel-server/--tunnel-token or --tunnel-alias flags target any other knot
server instead.`,
	MaxArgs: cli.NoArgs,
	Commands: []*cli.Command{
		spaceTunnelHttpCmd,
		spaceTunnelHttpsCmd,
		spaceTunnelStopCmd,
		spaceTunnelListCmd,
	},
}

// tunnelTargetFlags are the flags on `knot space tunnel http|https` that
// select which knot server the agent creates the tunnel on. They are named
// --tunnel-* because -s/-t/-a on `knot space` commands select the server the
// CLI itself talks to, which must be the space's own server here.
func tunnelTargetFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:  "tunnel-server",
			Usage: "Create the tunnel on this knot server instead of the space's own.",
		},
		&cli.StringFlag{
			Name:  "tunnel-token",
			Usage: "API token for --tunnel-server, must be valid on that server.",
		},
		&cli.StringFlag{
			Name:  "tunnel-alias",
			Usage: "Use this configured server alias (client.connection.<alias>) as the tunnel target.",
		},
		&cli.BoolFlag{
			Name:         "tunnel-tls-skip-verify",
			Usage:        "Skip TLS verification when the agent talks to the target server.",
			DefaultValue: true,
		},
	}
}

// resolveTunnelTarget resolves the --tunnel-* flags into the server/token the
// agent should create the tunnel on. An empty server means the space's own
// server.
func resolveTunnelTarget(cmd *cli.Command) (server, token string, skipVerify bool, err error) {
	skipVerify = cmd.GetBool("tunnel-tls-skip-verify")

	server, token = cmd.GetString("tunnel-server"), cmd.GetString("tunnel-token")
	switch {
	case server != "" && token != "":
		addr := config.NewServerAddr(server, token)
		return addr.HttpServer, addr.ApiToken, skipVerify, nil

	case server != "" || token != "":
		return "", "", false, fmt.Errorf("both --tunnel-server and --tunnel-token are required to target another server")
	}

	if cmd.HasFlag("tunnel-alias") {
		alias := cmd.GetString("tunnel-alias")
		cfg, ok := config.LookupServerAddr(alias, cmd)
		if !ok {
			return "", "", false, fmt.Errorf("no server configured for alias %q", alias)
		}
		return cfg.HttpServer, cfg.ApiToken, skipVerify, nil
	}

	return "", "", skipVerify, nil
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
		Flags:   tunnelTargetFlags(),
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
		return fmt.Errorf("invalid port %d: use a port between 1 and 65535", port)
	}

	name := cmd.GetStringArg("name")
	if !validate.Name(name) {
		return fmt.Errorf("invalid tunnel name %q: use lower-case letters, numbers and dashes only", name)
	}

	client, err := cmdutil.GetClient(cmd)
	if err != nil {
		return err
	}

	spaceId, err := resolveSpaceId(ctx, client, spaceName)
	if err != nil {
		return err
	}

	server, token, skipVerify, err := resolveTunnelTarget(cmd)
	if err != nil {
		return err
	}

	response, code, err := client.StartSpaceTunnel(ctx, spaceId, &apiclient.SpaceTunnelStartRequest{
		Protocol:            protocol,
		Port:                uint16(port),
		Name:                name,
		Server:              server,
		Token:               token,
		ServerTlsSkipVerify: skipVerify,
	})
	if err != nil {
		return spaceApiError(code, err, "start the tunnel", spaceName)
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
			return err
		}

		spaceId, err := resolveSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		code, err := client.StopSpaceTunnel(ctx, spaceId, &apiclient.SpaceTunnelStopRequest{Name: name})
		if err != nil {
			return spaceApiError(code, err, "stop the tunnel", spaceName)
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
			return err
		}

		spaceId, err := resolveSpaceId(ctx, client, spaceName)
		if err != nil {
			return err
		}

		response, code, err := client.ListSpaceTunnels(ctx, spaceId)
		if err != nil {
			return spaceApiError(code, err, "list tunnels", spaceName)
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
		return "", fmt.Errorf("couldn't list spaces: %w", err)
	}

	for _, s := range spaces.Spaces {
		if s.Name == spaceName {
			return s.Id, nil
		}
	}

	return "", fmt.Errorf("space %q not found", spaceName)
}

// spaceApiError describes a failed space-io API call. The status and the
// server's reason come from the wrapped HTTPError (the CLI's error formatter
// turns them into plain words and a hint); a 409 from these endpoints means
// the space isn't running, which gets a direct suggestion.
func spaceApiError(code int, err error, op string, spaceName string) error {
	if code == 409 {
		return fmt.Errorf("couldn't %s in space %q: the space isn't running; start it with `knot space start %s`", op, spaceName, spaceName)
	}
	return fmt.Errorf("couldn't %s in space %q: %w", op, spaceName, err)
}
