package connectcmd

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var ConnectListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List the known connections",
	Description: "Lists all the known connections stored in the local config.",
	Flags:       []cli.Flag{}, // No flags in the original command
	Run: func(ctx context.Context, cmd *cli.Command) error {
		// Get the connections from the config
		connections := cmd.ConfigFile.GetKeys("client")
		if len(connections) == 0 {
			fmt.Println("No connections found.")
			return nil
		}

		data := [][]string{}
		data = append(data, []string{"Alias", "Server"})

		for _, alias := range connections {
			server, _ := cmd.ConfigFile.GetValue(fmt.Sprintf("client.%s.server", alias))
			serverStr, _ := server.(string)
			data = append(data, []string{alias, serverStr})
		}

		util.PrintTable(data)
		return nil
	},
}
