package command

import (
	"os"

	agentCommands "github.com/paularlott/knot/agent/cmd"
	"github.com/paularlott/knot/agent/cmd/agentcmd"
	command_tunnel "github.com/paularlott/knot/agent/cmd/tunnel"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
)

var (
	RootCmd = &cobra.Command{
		Use:   "knot",
		Short: "knot simplifies the deployment of development environments",
		Long: `knot is a management tool for developer environments running within a Nomad cluster.

It offers both a user-friendly web interface and a command line interface to streamline the deployment process and simplify access.`,
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
	RootCmd.AddCommand(agentCommands.ConnectCmd)
	RootCmd.AddCommand(command_tunnel.TunnelCmd)
}

func initConfig() {
	config.InitConfig(RootCmd)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
