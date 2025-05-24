package connectcmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/pelletier/go-toml/v2"
)

var ConnectDeleteCmd = &cobra.Command{
	Use:   `delete <alias>`,
	Short: "Delete a given connection",
	Long:  `Deletes a connection stored in the local config.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		alias := args[0]

		// Get the connections from the config
		connections := viper.GetStringMap("client")
		if len(connections) == 0 {
			fmt.Println("No connections found.")
			return
		}

		// Check if the connection exists
		if _, exists := connections[alias]; !exists {
			fmt.Printf("Connection '%s' does not exist.\n", alias)
			os.Exit(1)
		}

		// Read the config file into a string
		configBytes, err := os.ReadFile(viper.ConfigFileUsed())
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

		// Delete the alias from the client section
		if clientMap, ok := config["client"].(map[string]interface{}); ok {
			delete(clientMap, alias)
		}

		// Marshal back to TOML
		result, err := toml.Marshal(config)
		if err != nil {
			fmt.Printf("Failed to encode modified config: %v\n", err)
			os.Exit(1)
		}

		// Write back to the file
		if err := os.WriteFile(viper.ConfigFileUsed(), result, 0644); err != nil {
			fmt.Printf("Failed to write modified config: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully deleted connection '%s'.\n", alias)
	},
}
