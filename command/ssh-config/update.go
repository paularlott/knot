package command_ssh_config

import (
	"context"
	"fmt"

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
				sshConfig += "  ProxyCommand knot forward ssh " + knotParams + space.Name + "\n"
			}
		}

		util.UpdateSSHConfig(sshConfig, alias)
		fmt.Println(".ssh/config has been updated")
		return nil
	},
}
