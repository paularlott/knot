package config

import (
	"fmt"
	"os"

	"github.com/spf13/viper"
)

func SaveConnection(alias string, server string, token string) error {
	if viper.ConfigFileUsed() == "" {
		// No config file so save this to the home folder
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}

		partial := viper.New()
		partial.Set("client."+alias+".server", server)
		partial.Set("client."+alias+".token", token)

		// Create any missing directories
		err = os.MkdirAll(home+"/.config/"+CONFIG_FILE_NAME, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		err = partial.WriteConfigAs(home + "/.config/" + CONFIG_FILE_NAME + "/" + CONFIG_FILE_NAME + "." + CONFIG_FILE_TYPE)
		if err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
	} else {
		partial := viper.New()
		partial.SetConfigFile(viper.ConfigFileUsed())
		err := partial.ReadInConfig()
		if err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}

		partial.Set("client."+alias+".server", server)
		partial.Set("client."+alias+".token", token)

		err = partial.WriteConfig()
		if err != nil {
			return fmt.Errorf("failed to save config file: %w", err)
		}
	}

	return nil
}
