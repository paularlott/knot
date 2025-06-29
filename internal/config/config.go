package config

import (
	"os"
	"strings"

	"github.com/paularlott/knot/internal/util"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const CONFIG_FILE_NAME = "knot"
const CONFIG_DOT_FILE_NAME = ".knot"
const CONFIG_FILE_TYPE = "toml"
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
	case "trace":
		zerolog.SetGlobalLevel(zerolog.TraceLevel)
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

	// Convert map[string]any to map[string][]string for DomainServers
	rawDomainServers := viper.GetStringMap("resolver.domains")
	domainServers := make(map[string][]string)
	for k, v := range rawDomainServers {
		switch vv := v.(type) {
		case []any:
			strSlice := make([]string, len(vv))
			for i, val := range vv {
				strSlice[i] = strings.TrimSpace(val.(string))
			}
			domainServers[k] = strSlice
		case string:
			domainServers[k] = []string{strings.TrimSpace(vv)}
		}
	}

	util.UpdateResolverConfig(&util.ResolverConfig{
		DefaultServers: viper.GetStringSlice("resolver.nameservers"),
		DomainServers:  domainServers,
	})
}
