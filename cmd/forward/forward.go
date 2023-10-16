package cmd_forward

import (
	"github.com/paularlott/knot/cmd"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  forwardCmd.PersistentFlags().StringP("nameserver", "n", "", "The nameserver to use for SRV lookups (default use system resolver).\nOverrides the " + cmd.CONFIG_ENV_PREFIX + "_NAMESERVER environment variable if set.")

  cmd.RootCmd.AddCommand(forwardCmd)
  forwardCmd.AddCommand(sshCmd)
  forwardCmd.AddCommand(portCmd)
}

var forwardCmd = &cobra.Command{
  Use:   "forward",
  Short: "Forward a connection to a service",
  Long:  "Forward a local connection to a remote service via a direct connection.",
  PersistentPreRun: func(ccmd *cobra.Command, args []string) {
    viper.BindPFlag("nameserver", ccmd.Flags().Lookup("nameserver"))
    viper.BindEnv("nameserver", cmd.CONFIG_ENV_PREFIX + "_NAMESERVER")
    viper.SetDefault("nameserver", "")
  },
  Run: func(cmd *cobra.Command, args []string) {
    cmd.Help()
    return
  },
}
