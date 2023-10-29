package commands_direct

import (
	"github.com/paularlott/knot/command"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  forwardCmd.PersistentFlags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the " + command.CONFIG_ENV_PREFIX + "_NAMESERVER environment variable if set.")

  command.RootCmd.AddCommand(forwardCmd)
  forwardCmd.AddCommand(sshCmd)
  forwardCmd.AddCommand(portCmd)
  forwardCmd.AddCommand(lookupCmd)
}

var forwardCmd = &cobra.Command{
  Use:   "direct",
  Short: "Direct connection to a service",
  Long:  "Create a direct connection from a local port to a remote service looking up the IP and port via SRV records.",
  PersistentPreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("nameserver", cmd.Flags().Lookup("nameserver"))
    viper.BindEnv("nameserver", command.CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("nameserver", "")
  },
  Run: func(cmd *cobra.Command, args []string) {
    cmd.Help()
  },
}
