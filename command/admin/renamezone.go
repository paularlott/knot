package commands_admin

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database"

	"github.com/paularlott/cli"
	"github.com/paularlott/gossip/hlc"
)

var RenameZoneCmd = &cli.Command{
	Name:  "rename-zone",
	Usage: "Rename a Zone",
	Description: `Rename a zone.

The zone name is updated within the database however spaces and volumes are not moved.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "old",
			Usage:    "The old zone name",
			Required: true,
		},
		&cli.StringArg{
			Name:     "new",
			Usage:    "The new zone name",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		oldZone := cmd.GetStringArg("old")
		newZone := cmd.GetStringArg("new")

		// Display what is going to happen and warning
		fmt.Println("Renaming zone", oldZone, "to", newZone)
		fmt.Print("This command will not move any spaces or volumes between zones.\n\n")

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to rename zone %s (yes/no): ", oldZone)
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Rename cancelled.")
			return nil
		}

		// Connect to the database
		db := database.GetInstance()

		// Load all volumes and update their zones
		fmt.Print("Updating volumes\n")
		volumes, err := db.GetVolumes()
		if err != nil {
			fmt.Println("Error getting volumes: ", err)
			return nil
		}

		for _, volume := range volumes {
			fmt.Print("Checking Volume: ", volume.Name)
			if volume.Zone == oldZone {
				volume.Zone = newZone
				volume.UpdatedAt = hlc.Now()
				err := db.SaveVolume(volume, []string{"Zone", "UpdatedAt"})
				if err != nil {
					fmt.Println("Error updating volume: ", err)
					return nil
				}
				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		// Load all spaces and update their zones
		fmt.Print("\nUpdating spaces\n")
		spaces, err := db.GetSpaces()
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return nil
		}

		for _, space := range spaces {
			fmt.Print("Checking Space: ", space.Name)
			if space.Zone == oldZone {
				space.Zone = newZone
				space.UpdatedAt = hlc.Now()
				err := db.SaveSpace(space, []string{"Zone", "UpdatedAt"})
				if err != nil {
					fmt.Println("Error updating space: ", err)
					return nil
				}
				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		fmt.Print("\nZone renamed\n")
		return nil
	},
}
