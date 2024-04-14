package commands_admin

import (
	"github.com/paularlott/knot/command"

	"github.com/spf13/cobra"
)

func init() {
	command.RootCmd.AddCommand(adminCmd)
	adminCmd.AddCommand(renameLocationCmd)
}

var adminCmd = &cobra.Command{
	Use:   "admin",
	Short: "Admin Operations",
	Long:  "Run administration operations for the server.",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
