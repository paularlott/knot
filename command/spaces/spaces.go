package command_spaces

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	spacesCmd.PersistentFlags().StringP("server", "s", "", "The address of the remote server to manage spaces on.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVER environment variable if set.")
	spacesCmd.PersistentFlags().StringP("token", "t", "", "The token to use for authentication.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TOKEN environment variable if set.")
	spacesCmd.PersistentFlags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")

	command.RootCmd.AddCommand(spacesCmd)
	spacesCmd.AddCommand(listCmd)
	spacesCmd.AddCommand(startCmd)
	spacesCmd.AddCommand(stopCmd)
	spacesCmd.AddCommand(createCmd)
	spacesCmd.AddCommand(deleteCmd)
}

var spacesCmd = &cobra.Command{
	Use:   "spaces",
	Short: "Manage spaces",
	Long:  "Manage your spaces from the command line.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("client.server", cmd.PersistentFlags().Lookup("server"))
		viper.BindEnv("client.server", config.CONFIG_ENV_PREFIX+"_SERVER")

		viper.BindPFlag("client.token", cmd.PersistentFlags().Lookup("token"))
		viper.BindEnv("client.token", config.CONFIG_ENV_PREFIX+"_TOKEN")

		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
