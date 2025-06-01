package commands_admin

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/paularlott/knot/database"

	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore <backupfile> [flags]",
	Short: "Restore the database",
	Long:  `Restore the database from a backup file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputFile := args[0]
		fmt.Println("Restoring database from file: ", inputFile)

		db := database.GetInstance()

		backupData := backupData{}

		// Load the backup file
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Println("Error loading backup file: ", err)
			os.Exit(1)
		}
		err = json.Unmarshal(data, &backupData)
		if err != nil {
			fmt.Println("Error unmarshalling backup file: ", err)
			os.Exit(1)
		}

		fmt.Println("Restoring templates...")
		for _, template := range backupData.Templates {
			err := db.SaveTemplate(template, nil)
			if err != nil {
				fmt.Println("Error restoring template: ", template.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored template: ", template.Name)
		}

		fmt.Println("Restoring template variables...")
		for _, variable := range backupData.TemplateVars {
			err := db.SaveTemplateVar(variable)
			if err != nil {
				fmt.Println("Error restoring template variable: ", variable.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored template variable: ", variable.Name)
		}

		fmt.Println("Restoring volumes...")
		for _, volume := range backupData.Volumes {
			err := db.SaveVolume(volume, nil)
			if err != nil {
				fmt.Println("Error restoring volume: ", volume.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored volume: ", volume.Name)
		}

		fmt.Println("Restoring groups...")
		for _, group := range backupData.Groups {
			err := db.SaveGroup(group)
			if err != nil {
				fmt.Println("Error restoring group: ", group.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored group: ", group.Name)
		}

		fmt.Println("Restoring roles...")
		for _, role := range backupData.Roles {
			err := db.SaveRole(role)
			if err != nil {
				fmt.Println("Error restoring role: ", role.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored role: ", role.Name)
		}

		fmt.Println("Restoring users...")
		for _, user := range backupData.Users {
			err := db.SaveUser(user.User, nil)
			if err != nil {
				fmt.Println("Error restoring user: ", user.User.Username, err)
				os.Exit(1)
			}
			fmt.Println("Restored user: ", user.User.Username)

			// Restore user tokens
			fmt.Println("Restoring tokens for user: ", user.User.Username)
			for _, token := range user.Tokens {
				err = db.SaveToken(token)
				if err != nil {
					fmt.Println("Error restoring token for user: ", user.User.Username, err)
					os.Exit(1)
				}
				fmt.Println("Restored token for user: ", user.User.Username, token.Name)
			}

			fmt.Println("Restored spaces for user: ", user.User.Username)
			for _, space := range user.Spaces {
				err := db.SaveSpace(space, nil)
				if err != nil {
					fmt.Println("Error restoring space: ", space.Name, err)
					os.Exit(1)
				}
				fmt.Println("Restored space: ", space.Name)
			}
		}

		fmt.Println("Database restore completed successfully.")
	},
}
