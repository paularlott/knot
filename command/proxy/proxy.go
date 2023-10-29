package command_proxy

import (
	"github.com/paularlott/knot/command"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
  proxyCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the " + command.CONFIG_ENV_PREFIX + "_SERVER environment variable if set.")

  command.RootCmd.AddCommand(proxyCmd)
  proxyCmd.AddCommand(sshCmd)
  proxyCmd.AddCommand(portCmd)
  proxyCmd.AddCommand(lookupCmd)
}

var proxyCmd = &cobra.Command{
  Use:   "proxy",
  Short: "Proxy a connection",
  Long:  "Proxy a connection from the local host to a remote destination via the proxy server.",
  PersistentPreRun: func(cmd *cobra.Command, args []string) {
    viper.BindPFlag("client.server", cmd.PersistentFlags().Lookup("server"))
    viper.BindEnv("client.server", command.CONFIG_ENV_PREFIX + "_SERVER")
  },
  Run: func(cmd *cobra.Command, args []string) {
    cmd.Help()
  },
}
