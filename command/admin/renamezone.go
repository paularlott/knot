package commands_admin

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/database"

	"github.com/spf13/cobra"
)

var renameZoneCmd = &cobra.Command{
	Use:   "rename-zone <old> <new> [flags]",
	Short: "Rename a zone",
	Long: `Rename a zone.

The zone name is updated within the database however spaces and volumes are not moved.

  old   The old zone name
  new   The new zone name`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {

		// Display what is going to happen and warning
		fmt.Println("Renaming zone ", args[0], "to", args[1])
		fmt.Print("This command will not move any spaces or volumes between zones.\n\n")

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to rename zone %s (yes/no): ", args[0])
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Rename cancelled.")
			return
		}

		// Connect to the database
		db := database.GetInstance()

		// Load all volumes and update their zones
		fmt.Print("Updating volumes\n")
		volumes, err := db.GetVolumes()
		if err != nil {
			fmt.Println("Error getting volumes: ", err)
			return
		}

		for _, volume := range volumes {
			fmt.Print("Checking Volume: ", volume.Name)
			if volume.Zone == args[0] {
				volume.Zone = args[1]
				volume.UpdatedAt = time.Now().UTC()
				err := db.SaveVolume(volume, []string{"Zone", "UpdatedAt"})
				if err != nil {
					fmt.Println("Error updating volume: ", err)
					return
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
			return
		}

		for _, space := range spaces {
			fmt.Print("Checking Space: ", space.Name)
			if space.Zone == args[0] {
				space.Zone = args[1]
				space.UpdatedAt = time.Now().UTC()
				err := db.SaveSpace(space, []string{"Zone", "UpdatedAt"})
				if err != nil {
					fmt.Println("Error updating space: ", err)
					return
				}

				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		fmt.Print("\nZone renamed\n")
	},
}
