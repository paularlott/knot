package connectcmd

import (
	"fmt"

	"github.com/paularlott/knot/internal/util"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var ConnectListCmd = &cobra.Command{
	Use:   `list`,
	Short: "List the known connections",
	Long:  `Lists all all the known connections stored in the local config.`,
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {

		// Get the connections from the config
		connections := viper.GetStringMap("client")
		if len(connections) == 0 {
			fmt.Println("No connections found.")
			return
		}

		data := [][]string{}
		data = append(data, []string{"Alias", "Server"})

		for alias := range connections {
			server := viper.GetString(fmt.Sprintf("client.%s.server", alias))
			data = append(data, []string{alias, server})
		}

		util.PrintTable(data)
	},
}
