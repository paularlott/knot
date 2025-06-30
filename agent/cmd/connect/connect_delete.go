package connectcmd

import (
	"context"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/paularlott/cli"
)

var ConnectDeleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete connection alias",
	Description: "Delete a given connection alias.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "alias",
			Usage:    "The alias to delete",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetStringArg("alias")

		// TODO rewrite this to use the config files

		// Get the config file path from the context or use default
		configFile := cmd.GetString("config")
		if configFile == "" {
			// fallback to default config file path if needed
			configFile = "config.toml"
		}

		// Read the config file into a string
		configBytes, err := os.ReadFile(configFile)
		if err != nil {
			fmt.Printf("Failed to read config file: %v\n", err)
			os.Exit(1)
		}

		// Parse the TOML into a map
		var config map[string]interface{}
		if err := toml.Unmarshal(configBytes, &config); err != nil {
			fmt.Printf("Failed to parse config file: %v\n", err)
			os.Exit(1)
		}

		// Get the connections from the config
		clientSection, ok := config["client"].(map[string]interface{})
		if !ok || len(clientSection) == 0 {
			fmt.Println("No connections found.")
			return nil
		}

		// Check if the connection exists
		if _, exists := clientSection[alias]; !exists {
			fmt.Printf("Connection '%s' does not exist.\n", alias)
			os.Exit(1)
		}

		// Delete the alias from the client section
		delete(clientSection, alias)

		// Marshal back to TOML
		result, err := toml.Marshal(config)
		if err != nil {
			fmt.Printf("Failed to encode modified config: %v\n", err)
			os.Exit(1)
		}

		// Write back to the file
		if err := os.WriteFile(configFile, result, 0644); err != nil {
			fmt.Printf("Failed to write modified config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully deleted connection '%s'.\n", alias)
		return nil
	},
}
