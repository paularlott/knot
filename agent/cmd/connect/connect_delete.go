package connectcmd

import (
	"context"
	"fmt"

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

		// Check if the config file is used
		if cmd.ConfigFile.FileUsed() == "" {
			return fmt.Errorf("No configuration file has been used.")
		}

		// Delete & Save
		cmd.ConfigFile.DeleteKey("client.connection." + alias)
		if err := cmd.ConfigFile.Save(); err != nil {
			return fmt.Errorf("Failed to save config file: %v", err)
		}

		fmt.Printf("Successfully deleted connection '%s'.\n", alias)
		return nil
	},
}
