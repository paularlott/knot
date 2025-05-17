package command_ssh_config

import (
	"fmt"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/util"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var sshConfigUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the .ssh/config file",
	Long:  `Update the .ssh/config file with the current live spaces that expose SSH.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		cfg := config.GetServerAddr(alias)
		client := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, viper.GetBool("tls_skip_verify"))

		// Get the current user
		user, err := client.WhoAmI()
		if err != nil {
			fmt.Println("Error getting user: ", err)
			return
		}

		spaces, _, err := client.GetSpaces(user.Id)
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
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
	},
}
