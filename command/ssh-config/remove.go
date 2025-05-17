package command_ssh_config

import (
	"fmt"

	"github.com/paularlott/knot/internal/util"

	"github.com/spf13/cobra"
)

var sshConfigRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove the config from .ssh/config",
	Long:  `Remove any knot space configurations from the .ssh/config file.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		alias, _ := cmd.Flags().GetString("alias")
		util.UpdateSSHConfig("", alias)
		fmt.Println(".ssh/config has been updated")
	},
}
