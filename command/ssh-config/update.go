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
	Name:  "update",
	Usage: "Update the .ssh/config file",
	Description: `Update the .ssh/config file with all spaces that expose SSH.

Spaces already in the file keep their agent forwarding setting, including changes made by hand. New spaces you own get agent forwarding as set by --agent-forwarding; spaces shared with you never get it. Use --reset to apply these defaults to every space again.`,
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "agent-forwarding",
			Usage:        "Enable SSH agent forwarding for new spaces you own.",
			ConfigPath:   []string{"ssh.agent_forwarding"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_SSH_AGENT_FORWARDING"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:  "reset",
			Usage: "Apply the agent forwarding defaults to spaces already in the file.",
		},
		&cli.StringFlag{
			Name:         "binary",
			Usage:        "Optional path to the knot binary.",
			DefaultValue: "knot",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("couldn't create the API client: %w", err)
		}

		// Get the current user
		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("couldn't get the current user: %w", err)
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id, false)
		if err != nil {
			return fmt.Errorf("couldn't list spaces: %w", err)
		}

		// If a config file was given then we need to use it
		configFile := ""
		if cmd.HasFlag("config") {
			absPath, err := filepath.Abs(cmd.GetString("config"))
			if err != nil {
				return fmt.Errorf("couldn't resolve the path of the SSH config file: %w", err)
			}
			configFile = " --config=" + absPath
		}

		// Spaces already in the file keep their agent forwarding setting
		existing := map[string]bool{}
		if !cmd.GetBool("reset") {
			existing, err = util.ReadSSHConfigAgentForwarding(alias)
			if err != nil {
				return fmt.Errorf("couldn't read the SSH config file: %w", err)
			}
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
			if space.TemplateHasSSH {
				host := "knot." + space.Name + machineAlias
				fmt.Println("Adding " + host + " to .ssh/config")

				// New spaces get forwarding only if they're our own
				forward, ok := existing[host]
				if !ok {
					forward = space.UserId == user.Id && cmd.GetBool("agent-forwarding")
				}

				sshConfig += "Host " + host + "\n"
				sshConfig += "  HostName " + host + "\n"
				sshConfig += "  StrictHostKeyChecking=no\n"
				sshConfig += "  LogLevel ERROR\n"
				sshConfig += "  UserKnownHostsFile=/dev/null\n"
				if forward {
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
