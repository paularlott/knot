package command

import (
	"fmt"

	"github.com/paularlott/knot/scaffold"

	"github.com/spf13/cobra"
)

func init() {
  scaffoldCmd.Flags().BoolP("server", "", false, "Generate a server configuration file")
  scaffoldCmd.Flags().BoolP("client", "", false, "Generate a client configuration file")

  RootCmd.AddCommand(scaffoldCmd)
}

var scaffoldCmd = &cobra.Command{
  Use:   "scaffold",
  Short: "Generate configuration files",
  Long:  `Generates example configuration files for use with knot.`,
  Args: cobra.NoArgs,
  PreRun: func(cmd *cobra.Command, args []string) {
    RootCmd.PersistentFlags().MarkHidden("config")
  },
  Run: func(cmd *cobra.Command, args []string) {
    if cmd.Flag("server").Value.String() == "true" {
      fmt.Println(scaffold.ServerScaffold)
    } else if cmd.Flag("client").Value.String() == "true" {
      fmt.Println(scaffold.ClientScaffold)
    } else {
      cmd.Help()
    }
  },
}
