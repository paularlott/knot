package command

import (
	"os"
	"strings"

	"github.com/paularlott/knot/agent/cmd/agentcmd"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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

	// TODO Remove this as it's just here as we move from JSON to msgpack
	RootCmd.PersistentFlags().BoolP("legacy", "", false, "Use legacy mode, json rather than msgpack for encoding/decoding.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_LEGACY environment variable if set.")

	RootCmd.AddCommand(agentcmd.AgentCmd)
}

func initConfig() {
	config.InitConfig(RootCmd)
}

func Execute() {
	if err := RootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

type ServerAddr struct {
	HttpServer string
	WsServer   string
	ApiToken   string
}

// Read the server configuration information and generate the websocket address
func GetServerAddr() ServerAddr {
	flags := ServerAddr{}

	flags.HttpServer = viper.GetString("client.server")
	flags.ApiToken = viper.GetString("client.token")

	// If flags.server empty then throw and error
	if flags.HttpServer == "" {
		cobra.CheckErr("Missing proxy server address")
	}

	if flags.ApiToken == "" {
		cobra.CheckErr("Missing API token")
	}

	if !strings.HasPrefix(flags.HttpServer, "http://") && !strings.HasPrefix(flags.HttpServer, "https://") {
		flags.HttpServer = "https://" + flags.HttpServer
	}

	// Fix up the address to a websocket address
	flags.HttpServer = strings.TrimSuffix(flags.HttpServer, "/")
	flags.WsServer = "ws" + flags.HttpServer[4:]

	return flags
}
