package command_proxy

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	proxyCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to proxy through.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVER environment variable if set.")
	proxyCmd.PersistentFlags().StringP("token", "t", "", "The token to use for authentication.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TOKEN environment variable if set.")

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
		viper.BindEnv("client.server", config.CONFIG_ENV_PREFIX+"_SERVER")

		viper.BindPFlag("client.token", cmd.PersistentFlags().Lookup("token"))
		viper.BindEnv("client.token", config.CONFIG_ENV_PREFIX+"_TOKEN")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
