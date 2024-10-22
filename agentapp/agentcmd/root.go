package agentcmd

import (
	"os"
	"strconv"
	"strings"

	"github.com/paularlott/knot/build"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const CONFIG_FILE_NAME = ".knot"
const CONFIG_FILE_TYPE = "yaml"
const CONFIG_ENV_PREFIX = "KNOT"

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

	RootCmd.PersistentFlags().StringP("config", "c", "", "Config file (default is "+CONFIG_FILE_NAME+"."+CONFIG_FILE_TYPE+" in the current directory or $HOME/).\nOverrides the "+CONFIG_ENV_PREFIX+"_CONFIG environment variable if set.")
	RootCmd.PersistentFlags().StringP("log-level", "", "info", "Log level (debug, info, warn, error, fatal, panic).\nOverrides the "+CONFIG_ENV_PREFIX+"_LOGLEVEL environment variable if set.")
}

func initConfig() {
	// Find home directory.
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)

	// Search config in home directory with name ".knot" (without extension).
	viper.AddConfigPath(".")
	viper.AddConfigPath(home)
	viper.SetConfigName(CONFIG_FILE_NAME) // Name of config file without extension
	viper.SetConfigType(CONFIG_FILE_TYPE) // Type of config file
	viper.SetEnvPrefix(CONFIG_ENV_PREFIX)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	//viper.AutomaticEnv() // Read in environment variables that match

	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindEnv("config", CONFIG_ENV_PREFIX+"_CONFIG")
	viper.BindPFlag("log.level", RootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindEnv("log.level", CONFIG_ENV_PREFIX+"_LOGLEVEL")

	// If config file given then use it
	cfgFile := viper.GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	}

	viper.ReadInConfig()

	switch viper.GetString("log.level") {
	case "debug":
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	case "info":
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	case "warn":
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	case "error":
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	default:
		zerolog.SetGlobalLevel(zerolog.WarnLevel)
	}
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

func FixListenAddress(address string) string {
	if address == "" {
		return ""
	}

	// If the address is just numbers then assume it's a port and prefix with a colon
	if _, err := strconv.Atoi(address); err == nil {
		return ":" + address
	}

	return address
}
