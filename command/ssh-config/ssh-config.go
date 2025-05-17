package command_ssh_config

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	sshConfigCmd.PersistentFlags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")
	sshConfigCmd.PersistentFlags().StringP("alias", "a", "default", "The server alias to use.")

	command.RootCmd.AddCommand(sshConfigCmd)
	sshConfigCmd.AddCommand(sshConfigUpdateCmd)
	sshConfigCmd.AddCommand(sshConfigRemoveCmd)
}

var sshConfigCmd = &cobra.Command{
	Use:   "ssh-config",
	Short: "Operate on the .ssh/config file",
	Long:  `Operations to perform management of the .ssh/config file.`,
	Args:  cobra.NoArgs,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
