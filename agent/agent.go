package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	agent_cmd "github.com/paularlott/knot/agent/cmd"
	"github.com/paularlott/knot/agent/cmd/agentcmd"
	command_tunnel "github.com/paularlott/knot/agent/cmd/tunnel"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC822})
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	var configFile = config.CONFIG_FILE

	cmd := &cli.Command{
		Name:  "knot",
		Usage: "knot agent to connect the space to the knot server",
		Description: `knot is a management tool for developer environments running within a Nomad cluster.

The agent connects environments to the knot server.`,
		Version: build.Version,
		ConfigFile: cli_toml.NewConfigFile(&configFile, func() []string {
			paths := []string{"."}

			home, err := os.UserHomeDir()
			if err == nil {
				paths = append(paths, home)
			}

			paths = append(paths, filepath.Join(home, ".config", config.CONFIG_DIR))

			return paths
		}),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Aliases:     []string{"c"},
				Usage:       "Name and path to the configuration file to use.",
				DefaultText: config.CONFIG_FILE + " in the current directory, $HOME/ or $HOME/.config/" + config.CONFIG_DIR + "/" + config.CONFIG_FILE,
				EnvVars:     []string{config.CONFIG_ENV_PREFIX + "_CONFIG"},
				AssignTo:    &configFile,
				Global:      true,
			},
			&cli.StringFlag{
				Name:         "log-level",
				Usage:        "Log level one of trace, debug, info, warn, error, fatal, panic",
				ConfigPath:   []string{"log.level"},
				EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LOGLEVEL"},
				DefaultValue: "info",
				Global:       true,
			},
			&cli.StringSliceFlag{
				Name:       "nameservers",
				Usage:      "Nameservers to use for DNS resolution, maybe given multiple times.",
				ConfigPath: []string{"resolver.nameservers"},
				EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_NAMESERVERS"},
				Global:     true,
			},
		},
		Commands: []*cli.Command{
			agent_cmd.ConnectCmd,
			agentcmd.AgentCmd,
			command_tunnel.TunnelCmd,
		},
		PreRun: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			config.InitCommonConfig(cmd)
			return ctx, nil
		},
	}

	err := cmd.Execute(context.Background())
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}

	os.Exit(0)
}
