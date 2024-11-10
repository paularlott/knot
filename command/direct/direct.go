package commands_direct

import (
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	directCmd.PersistentFlags().StringSliceP("nameserver", "", []string{}, "The address of the nameserver to use for SRV lookups, can be given multiple times (default use system resolver).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_NAMESERVERS environment variable if set.")

	command.RootCmd.AddCommand(directCmd)
	directCmd.AddCommand(sshCmd)
	directCmd.AddCommand(portCmd)
	directCmd.AddCommand(lookupCmd)
}

var directCmd = &cobra.Command{
	Use:   "direct",
	Short: "Direct connection to a service",
	Long:  "Create a direct connection from a local port to a remote service looking up the IP and port via SRV records.",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("resolver.nameservers", cmd.Flags().Lookup("nameserver"))
		viper.BindEnv("resolver.nameservers", config.CONFIG_ENV_PREFIX+"_NAMESERVERS")
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}
