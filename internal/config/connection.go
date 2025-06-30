package config

import (
	"fmt"
	"os"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

func SaveConnection(alias string, server string, token string, cmd *cli.Command) error {
	if cmd.ConfigFile.FileUsed() == "" {
		// No config file so save this to the home folder
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %w", err)
		}

		newCfg := cli_toml.NewConfigFile(cli.StrToPtr(home+"/.config/"+CONFIG_DIR+"/"+CONFIG_FILE), nil)

		newCfg.SetValue("client."+alias+".server", server)
		newCfg.SetValue("client."+alias+".token", token)

		// Create any missing directories
		err = os.MkdirAll(home+"/.config/"+CONFIG_DIR, os.ModePerm)
		if err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		err = newCfg.Save()
		if err != nil {
			return fmt.Errorf("failed to create config file: %w", err)
		}
	} else {
		cmd.ConfigFile.SetValue("client."+alias+".server", server)
		cmd.ConfigFile.SetValue("client."+alias+".token", token)

		err := cmd.ConfigFile.Save()
		if err != nil {
			return fmt.Errorf("failed to save config file: %w", err)
		}
	}

	return nil
}
