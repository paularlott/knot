package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/paularlott/knot/agent/cmd/agentcmd"
	command_tunnel "github.com/paularlott/knot/agent/cmd/tunnel"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/command"
	commands_admin "github.com/paularlott/knot/command/admin"
	commands_direct "github.com/paularlott/knot/command/direct"
	commands_forward "github.com/paularlott/knot/command/forward"
	command_proxy "github.com/paularlott/knot/command/proxy"
	command_spaces "github.com/paularlott/knot/command/spaces"
	command_ssh_config "github.com/paularlott/knot/command/ssh-config"
	command_templates "github.com/paularlott/knot/command/templates"
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
		Usage: "knot simplifies the deployment of development environments",
		Description: `knot is a management tool for developer environments running within a Nomad cluster.

It offers both a user-friendly web interface and a command line interface to streamline the deployment process and simplify access.`,
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
				Name:     "config",
				Aliases:  []string{"c"},
				Usage:    "Config file (default is " + config.CONFIG_FILE_NAME + "." + config.CONFIG_FILE_TYPE + " in the current directory or $HOME/).",
				AssignTo: &configFile,
				EnvVars:  []string{config.CONFIG_ENV_PREFIX + "_CONFIG"},
				Global:   true,
			},
			&cli.StringFlag{
				Name:         "log-level",
				Usage:        "Log level (debug, info, warn, error, fatal, panic).",
				ConfigPath:   []string{"log.level"},
				EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_LOGLEVEL"},
				DefaultValue: "info",
				Global:       true,
			},
		},
		Commands: []*cli.Command{
			command.ConnectCmd,
			commands_forward.ForwardCmd,
			command_spaces.SpacesCmd,
			command_ssh_config.SshConfigCmd,
			command_templates.TemplatesCmd,
			command_proxy.ProxyCmd,
			agentcmd.AgentCmd,
			commands_admin.AdminCmd,
			command_tunnel.TunnelCmd,
			command.ServerCmd,
			command.PingCmd,
			commands_direct.DirectCmd,
			command.ScaffoldCmd,
			command.GenkeyCmd,
			command.LegalCmd,
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
