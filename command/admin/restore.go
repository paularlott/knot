package commands_admin

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/spf13/cobra"
)

func init() {
	restoreCmd.Flags().StringP("encrypt-key", "e", "", "Encrypt the backup file with the given key. The key must be 32 bytes long.")
}

var restoreCmd = &cobra.Command{
	Use:   "restore <backupfile> [flags]",
	Short: "Restore the database",
	Long:  `Restore the database from a backup file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		inputFile := args[0]

		key, _ := cmd.Flags().GetString("encrypt-key")
		if key != "" && len(key) != 32 {
			fmt.Println("Error: Encrypt key must be 32 bytes long.")
			os.Exit(1)
		}

		fmt.Println("Restoring database from file: ", inputFile)

		db := database.GetInstance()

		backupData := backupData{}

		// Load the backup file
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Println("Error loading backup file: ", err)
			os.Exit(1)
		}

		if key != "" {
			// Decrypt the backup file
			data = []byte(crypt.Decrypt(key, string(data)))
		}

		err = json.Unmarshal(data, &backupData)
		if err != nil {
			fmt.Println("Error unmarshalling backup file: ", err)
			os.Exit(1)
		}

		fmt.Println("Restoring audit logs...")
		for _, auditLog := range backupData.AuditLogs {
			err := db.SaveAuditLog(auditLog)
			if err != nil {
				fmt.Println("Error restoring audit log: ", auditLog.Event, err)
				os.Exit(1)
			}
			fmt.Println("Restored audit log: ", auditLog.Event)
		}

		fmt.Println("Resotring configuration values...")
		for _, cfgValue := range backupData.CfgValues {
			err := db.SaveCfgValue(cfgValue)
			if err != nil {
				fmt.Println("Error restoring configuration value: ", cfgValue.Name, err)
				os.Exit(1)
			}
			fmt.Println("Restored configuration value: ", cfgValue.Name)
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

			fmt.Println("Restoring spaces for user: ", user.User.Username)
			for _, space := range user.Spaces {
				// If started at isn't set then use now
				if space.StartedAt.IsZero() {
					space.StartedAt = time.Now().UTC()
				}

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
