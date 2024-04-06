package command_ssh_config

import (
	"github.com/paularlott/knot/command"

	"github.com/spf13/cobra"
)

func init() {
	command.RootCmd.AddCommand(sshConfigCmd)

	sshConfigCmd.AddCommand(sshConfigUpdateCmd)
	sshConfigCmd.AddCommand(sshConfigRemoveCmd)
}

var sshConfigCmd = &cobra.Command{
	Use:   "ssh-config",
	Short: "Operate on the .ssh/config file",
	Long:  `Operations to perform management of the .ssh/config file.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
