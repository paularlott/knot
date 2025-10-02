package command

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/scaffold"

	"github.com/paularlott/cli"
)

var ScaffoldCmd = &cli.Command{
	Name:        "scaffold",
	Usage:       "Generate configuration files",
	Description: "Generates example configuration files for use with knot.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:       "server",
			Usage:      "Generate a server configuration file",
			ConfigPath: []string{"scaffold.server"},
		},
		&cli.BoolFlag{
			Name:       "client",
			Usage:      "Generate a client configuration file",
			ConfigPath: []string{"scaffold.client"},
		},
		&cli.BoolFlag{
			Name:       "agent",
			Usage:      "Generate an agent configuration file",
			ConfigPath: []string{"scaffold.agent"},
		},
		&cli.BoolFlag{
			Name:       "nomad",
			Usage:      "Generate a nomad job file",
			ConfigPath: []string{"scaffold.nomad"},
		},
		&cli.BoolFlag{
			Name:       "system-prompt",
			Usage:      "Generate the internal system prompt",
			ConfigPath: []string{"scaffold.system_prompt"},
		},
		&cli.BoolFlag{
			Name:       "nomad-spec",
			Usage:      "Generate the internal nomad spec",
			ConfigPath: []string{"scaffold.nomad_spec"},
		},
		&cli.BoolFlag{
			Name:       "docker-spec",
			Usage:      "Generate the internal docker spec",
			ConfigPath: []string{"scaffold.docker_spec"},
		},
		&cli.BoolFlag{
			Name:       "podman-spec",
			Usage:      "Generate the internal podman spec",
			ConfigPath: []string{"scaffold.podman_spec"},
		},
		&cli.BoolFlag{
			Name:       "apple-spec",
			Usage:      "Generate the internal apple spec",
			ConfigPath: []string{"scaffold.apple_spec"},
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		any := false

		if cmd.GetBool("server") {
			fmt.Println(scaffold.ServerScaffold)
			any = true
		}
		if cmd.GetBool("client") {
			fmt.Println(scaffold.ClientScaffold)
			any = true
		}
		if cmd.GetBool("agent") {
			fmt.Println(scaffold.AgentScaffold)
			any = true
		}
		if cmd.GetBool("nomad") {
			fmt.Println(scaffold.NomadScaffold)
			any = true
		}
		if cmd.GetBool("system-prompt") {
			fmt.Println(scaffold.GetSystemPromptScaffold())
			any = true
		}
		if cmd.GetBool("nomad-spec") {
			fmt.Println(scaffold.GetNomadSpecScaffold())
			any = true
		}
		if cmd.GetBool("docker-spec") {
			fmt.Println(scaffold.GetDockerSpecScaffold())
			any = true
		}
		if cmd.GetBool("podman-spec") {
			fmt.Println(scaffold.GetPodmanSpecScaffold())
			any = true
		}
		if cmd.GetBool("apple-spec") {
			fmt.Println(scaffold.GetAppleSpecScaffold())
			any = true
		}

		if !any {
			cmd.ShowHelp()
		}
		return nil
	},
}
