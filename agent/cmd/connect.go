package agentcmd

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	RootCmd.AddCommand(ConnectCmd)
}

var ConnectCmd = &cobra.Command{
	Use:   `connect`,
	Short: "Generate API key",
	Long:  `Asks the running agent to generate a new API key on the server and stores it in the local config.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		var response agentlink.ConnectResponse

		err := agentlink.SendWithResponseMsg(agentlink.CommandConnect, nil, &response)
		if err != nil {
			fmt.Println("Unable to connect to the agent, please check that the agent is running.")
			os.Exit(1)
		}

		if !response.Success {
			fmt.Println("Failed to create an API token")
			os.Exit(1)
		}

		if viper.ConfigFileUsed() == "" {
			// No config file so save this to the home folder
			home, err := os.UserHomeDir()
			cobra.CheckErr(err)

			partial := viper.New()
			partial.Set("client.default.server", response.Server)
			partial.Set("client.default.token", response.Token)

			// Create any missing directories
			err = os.MkdirAll(home+"/.config/"+config.CONFIG_FILE_NAME, os.ModePerm)
			if err != nil {
				fmt.Println("Failed to create config directory")
				os.Exit(1)
			}

			err = partial.WriteConfigAs(home + "/.config/" + config.CONFIG_FILE_NAME + "/" + config.CONFIG_FILE_NAME + "." + config.CONFIG_FILE_TYPE)
			if err != nil {
				fmt.Println("Failed to create config file")
				os.Exit(1)
			}
		} else {
			partial := viper.New()
			partial.SetConfigFile(viper.ConfigFileUsed())
			err = partial.ReadInConfig()
			if err != nil {
				fmt.Println("Failed to read config file")
				os.Exit(1)
			}

			partial.Set("client.default.server", response.Server)
			partial.Set("client.default.token", response.Token)

			err = partial.WriteConfig()
			if err != nil {
				fmt.Println("Failed to save config file")
				os.Exit(1)
			}
		}

		fmt.Println("Successfully created API token")
	},
}
