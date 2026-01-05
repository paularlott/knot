package command_ssh_config

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var SshConfigUpdateCmd = &cli.Command{
	Name:        "update",
	Usage:       "Update the .ssh/config file",
	Description: "Update the .ssh/config file with the current live spaces that expose SSH.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "agent-forwarding",
			Usage:        "Enable SSH Agent Forwarding.",
			ConfigPath:   []string{"ssh.agent_forwarding"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_SSH_AGENT_FORWARDING"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:         "binary",
			Usage:        "Option path to the knot binary.",
			DefaultValue: "knot",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		// If a config file was given then we need to use it
		configFile := ""
		if cmd.HasFlag("config") {
			absPath, err := filepath.Abs(cmd.GetString("config"))
			if err != nil {
				return fmt.Errorf("Failed to resolve absolute path for config file: %w", err)
			}
			configFile = " --config=" + absPath
		}

		// For all spaces query the service state and build a list of those that are deployed and have SSH exposed
		sshConfig := ""
		knotParams := ""
		machineAlias := alias
		if machineAlias == "default" {
			machineAlias = ""
		} else {
			machineAlias = "." + machineAlias
			knotParams = "--alias " + alias + " "
		}
		for _, space := range spaces.Spaces {
			if space.IsDeployed && space.HasSSH {
				fmt.Println("Adding knot." + space.Name + machineAlias + " to .ssh/config")

				sshConfig += "Host knot." + space.Name + machineAlias + "\n"
				sshConfig += "  HostName knot." + space.Name + machineAlias + "\n"
				sshConfig += "  StrictHostKeyChecking=no\n"
				sshConfig += "  LogLevel ERROR\n"
				sshConfig += "  UserKnownHostsFile=/dev/null\n"
				if cmd.GetBool("agent-forwarding") {
					sshConfig += "  ForwardAgent yes\n"
				}
				sshConfig += "  ProxyCommand " + cmd.GetString("binary") + " forward ssh " + knotParams + space.Name + configFile + "\n"
			}
		}

		util.UpdateSSHConfig(sshConfig, alias)
		fmt.Println(".ssh/config has been updated")
		return nil
	},
}
