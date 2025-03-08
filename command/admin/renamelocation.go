package commands_admin

import (
	"fmt"

	"github.com/paularlott/knot/database"

	"github.com/spf13/cobra"
)

var renameLocationCmd = &cobra.Command{
	Use:   "rename-location <old> <new> [flags]",
	Short: "Rename a location",
	Long: `Rename a location.

The location name is updated within the database however spaces and volumes are not moved.

  old   The old location name
  new   The new location name`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {

		// Display what is going to happen and warning
		fmt.Println("Renaming location ", args[0], "to", args[1])
		fmt.Print("This command will not move any spaces or volumes between locations.\n\n")

		// Prompt the user to confirm the deletion
		var confirm string
		fmt.Printf("Are you sure you want to rename location %s (yes/no): ", args[0])
		fmt.Scanln(&confirm)
		if confirm != "yes" {
			fmt.Println("Rename cancelled.")
			return
		}

		// Connect to the database
		db := database.GetInstance()

		// Load all volumes and update their locations
		fmt.Print("Updating volumes\n")
		volumes, err := db.GetVolumes()
		if err != nil {
			fmt.Println("Error getting volumes: ", err)
			return
		}

		for _, volume := range volumes {
			fmt.Print("Checking Volume: ", volume.Name)
			if volume.Location == args[0] {
				volume.Location = args[1]
				err := db.SaveVolume(volume, []string{"Location"})
				if err != nil {
					fmt.Println("Error updating volume: ", err)
					return
				}

				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		// Load all spaces and update their locations
		fmt.Print("\nUpdating spaces\n")
		spaces, err := db.GetSpaces()
		if err != nil {
			fmt.Println("Error getting spaces: ", err)
			return
		}

		for _, space := range spaces {
			fmt.Print("Checking Space: ", space.Name)
			if space.Location == args[0] {
				space.Location = args[1]
				err := db.SaveSpace(space, []string{"Location"})
				if err != nil {
					fmt.Println("Error updating space: ", err)
					return
				}

				fmt.Print(" - Updated\n")
			} else {
				fmt.Print(" - Skipping\n")
			}
		}

		fmt.Print("\nLocation renamed\n")
	},
}
