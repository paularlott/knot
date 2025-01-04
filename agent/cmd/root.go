package agentcmd

import (
	"os"

	"github.com/paularlott/knot/agent/cmd/agentcmd"
	tunnelcmd "github.com/paularlott/knot/agent/cmd/tunnel"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
)

var (
	RootCmd = &cobra.Command{
		Use:   "knot-agent",
		Short: "knot agent to connect the environment to the knot server",
		Long: `knot is a management tool for developer environments running within a Nomad cluster.

The agent connects environments to the knot server.`,
		Version: build.Version,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringP("config", "c", "", "Config file (default is "+config.CONFIG_FILE_NAME+"."+config.CONFIG_FILE_TYPE+" in the current directory or $HOME/).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_CONFIG environment variable if set.")
	RootCmd.PersistentFlags().StringP("log-level", "", "info", "Log level (debug, info, warn, error, fatal, panic).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LOGLEVEL environment variable if set.")

	RootCmd.AddCommand(agentcmd.AgentCmd)
	RootCmd.AddCommand(tunnelcmd.TunnelCmd)
}

func initConfig() {
	config.InitConfig(RootCmd)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
