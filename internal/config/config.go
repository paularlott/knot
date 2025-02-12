package config

import (
	"os"
	"strings"

	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const CONFIG_FILE_NAME = "knot"
const CONFIG_DOT_FILE_NAME = ".knot"
const CONFIG_FILE_TYPE = "yaml"
const CONFIG_ENV_PREFIX = "KNOT"

func InitConfig(root *cobra.Command) {
	// Find home directory.
	home, err := os.UserHomeDir()
	cobra.CheckErr(err)

	// Search config in home directory with name ".knot" (without extension).
	viper.AddConfigPath(".")
	viper.AddConfigPath(home)
	viper.AddConfigPath(home + "/.config/" + CONFIG_FILE_NAME)
	viper.SetConfigName(CONFIG_DOT_FILE_NAME) // Name of config file without extension
	viper.SetConfigType(CONFIG_FILE_TYPE)     // Type of config file
	viper.SetEnvPrefix(CONFIG_ENV_PREFIX)
	viper.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	//viper.AutomaticEnv() // Read in environment variables that match

	viper.BindPFlag("config", root.PersistentFlags().Lookup("config"))
	viper.BindEnv("config", CONFIG_ENV_PREFIX+"_CONFIG")
	viper.BindPFlag("log.level", root.PersistentFlags().Lookup("log-level"))
	viper.BindEnv("log.level", CONFIG_ENV_PREFIX+"_LOGLEVEL")

	// If config file given then use it
	cfgFile := viper.GetString("config")
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
		viper.ReadInConfig()
	} else {
		if err := viper.ReadInConfig(); err != nil {
			viper.SetConfigName(CONFIG_FILE_NAME)
			viper.ReadInConfig()
		}
	}

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
