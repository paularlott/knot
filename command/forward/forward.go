package commands_forward

import (
	"github.com/paularlott/knot/command"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  forwardCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_SERVER environment variable if set.")
  forwardCmd.PersistentFlags().StringP("token", "t", "", "The token to use for authentication.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_TOKEN environment variable if set.")

  command.RootCmd.AddCommand(forwardCmd)
  forwardCmd.AddCommand(sshCmd)
  forwardCmd.AddCommand(portCmd)
}

var forwardCmd = &cobra.Command{
  Use:   "forward",
  Short: "Forward a connection via the agent service",
  Long:  "Forward a local connection to a remote server via the agent service.",
  PersistentPreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("client.server", cmd.PersistentFlags().Lookup("server"))
    viper.BindEnv("client.server", command.CONFIG_ENV_PREFIX + "_SERVER")

    viper.BindPFlag("client.token", cmd.PersistentFlags().Lookup("token"))
    viper.BindEnv("client.token", command.CONFIG_ENV_PREFIX + "_TOKEN")
  },
  Run: func(cmd *cobra.Command, args []string) {
    cmd.Help()
  },
}
