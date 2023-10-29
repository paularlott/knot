package command

import (
	"os"
	"strings"

	"github.com/paularlott/knot/build"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const CONFIG_FILE_NAME = ".knot"
const CONFIG_FILE_TYPE = "yaml"
const CONFIG_ENV_PREFIX = "KNOT"

var (
  RootCmd = &cobra.Command{
    Use:   "knot",
    Short: "knot is a proxy server using WebSockets to tunnel SSH and TCP connections",
    Long: `Currently a proxy server and client that can use WebSockets to tunnel SSH and TCP connections between a local and remote system over WebSockets.

It also helps with direct access to services identified by SRV records.`,
    Version: build.Version + " (" + build.Date + ")",
    PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
    },
    Run: func(cmd *cobra.Command, args []string) {
      cmd.Help()
    },
  }
)

func init() {
  cobra.OnInitialize(initConfig)

  RootCmd.PersistentFlags().StringP("config", "c", "", "Config file (default is " + CONFIG_FILE_NAME + "." + CONFIG_FILE_TYPE + " in the current directory or $HOME/).\nOverrides the " + CONFIG_ENV_PREFIX + "_CONFIG environment variable if set.")
  viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
  viper.BindEnv("config", CONFIG_ENV_PREFIX + "_CONFIG")

  RootCmd.PersistentFlags().StringP("loglevel", "", "info", "Log level (debug, info, warn, error, fatal, panic).\nOverrides the " + CONFIG_ENV_PREFIX + "_LOGLEVEL environment variable if set.")
  viper.BindPFlag("log.level", RootCmd.PersistentFlags().Lookup("loglevel"))
  viper.BindEnv("log.level", CONFIG_ENV_PREFIX + "_LOGLEVEL")
}

func initConfig() {
  cfgFile := viper.GetString("config")

  if cfgFile != "" {
    // Use config file from the flag
    viper.SetConfigFile(cfgFile)

    if err := viper.ReadInConfig(); err != nil {
      log.Fatal().Msgf("Missing config file: %s", viper.ConfigFileUsed())
    }
  } else {
    // Find home directory.
    home, err := os.UserHomeDir()
    cobra.CheckErr(err)

    // Search config in home directory with name ".knot" (without extension).
    viper.AddConfigPath(".")
    viper.AddConfigPath(home)
    viper.SetConfigName(CONFIG_FILE_NAME) // Name of config file without extension
    viper.SetConfigType(CONFIG_FILE_TYPE) // Type of config file
  }

  viper.SetEnvPrefix(CONFIG_ENV_PREFIX)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
  //viper.AutomaticEnv() // Read in environment variables that match

  viper.ReadInConfig()
}

func Execute() {
  if err := RootCmd.Execute(); err != nil {
    os.Exit(1)
  }
}

type ServerAddr struct {
  HttpServer string
  WsServer string
}

// Read the server configuration information and generate the websocket address
func GetServerAddr() ServerAddr {
  flags := ServerAddr{}

  flags.HttpServer = viper.GetString("client.server")

  // If flags.server empty then throw and error
  if flags.HttpServer == "" {
    cobra.CheckErr("Missing proxy server address")
  }

  // Fix up the address to a websocket address
  flags.HttpServer = strings.TrimSuffix(flags.HttpServer, "/")
  if strings.HasPrefix(flags.HttpServer, "http") {
    flags.WsServer = "ws" + flags.HttpServer[4:]
  } else {
    flags.WsServer = "ws://" + flags.HttpServer
  }

  return flags
}
