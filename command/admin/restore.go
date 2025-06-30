package commands_admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/cli"
)

var RestoreCmd = &cli.Command{
	Name:        "restore",
	Usage:       "Restore a backup file",
	Description: "Restore the database from a backup file.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "backupfile",
			Usage:    "The name of the backup file to restore",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "encrypt-key",
			Aliases: []string{"e"},
			Usage:   "Encrypt the backup file with the given key. The key must be 32 bytes long.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_RESTORE_ENCRYPT_KEY"},
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		inputFile := cmd.GetStringArg("backupfile")
		key := cmd.GetString("encrypt-key")
		if key != "" && len(key) != 32 {
			return fmt.Errorf("Error: Encrypt key must be 32 bytes long.")
		}

		fmt.Println("Restoring database from file: ", inputFile)
		db := database.GetInstance()
		backupData := backupData{}

		// Load the backup file
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("Error loading backup file: %w", err)
		}

		if key != "" {
			// Decrypt the backup file
			data = []byte(crypt.Decrypt(key, string(data)))
		}

		err = json.Unmarshal(data, &backupData)
		if err != nil {
			return fmt.Errorf("Error unmarshalling backup file: %w", err)
		}

		fmt.Println("Restoring audit logs...")
		for _, auditLog := range backupData.AuditLogs {
			err := db.SaveAuditLog(auditLog)
			if err != nil {
				return fmt.Errorf("Error restoring audit log: %w", err)
			}
			fmt.Println("Restored audit log: ", auditLog.Event)
		}

		fmt.Println("Restoring configuration values...")
		for _, cfgValue := range backupData.CfgValues {
			err := db.SaveCfgValue(cfgValue)
			if err != nil {
				return fmt.Errorf("Error restoring configuration value: %w", err)
			}
			fmt.Println("Restored configuration value: ", cfgValue.Name)
		}

		fmt.Println("Restoring templates...")
		for _, template := range backupData.Templates {
			err := db.SaveTemplate(template, nil)
			if err != nil {
				return fmt.Errorf("Error restoring template: %w", err)
			}
			fmt.Println("Restored template: ", template.Name)
		}

		fmt.Println("Restoring template variables...")
		for _, variable := range backupData.TemplateVars {
			err := db.SaveTemplateVar(variable)
			if err != nil {
				return fmt.Errorf("Error restoring template variable: %w", err)
			}
			fmt.Println("Restored template variable: ", variable.Name)
		}

		fmt.Println("Restoring volumes...")
		for _, volume := range backupData.Volumes {
			err := db.SaveVolume(volume, nil)
			if err != nil {
				return fmt.Errorf("Error restoring volume: %w", err)
			}
			fmt.Println("Restored volume: ", volume.Name)
		}

		fmt.Println("Restoring groups...")
		for _, group := range backupData.Groups {
			err := db.SaveGroup(group)
			if err != nil {
				return fmt.Errorf("Error restoring group: %w", err)
			}
			fmt.Println("Restored group: ", group.Name)
		}

		fmt.Println("Restoring roles...")
		for _, role := range backupData.Roles {
			err := db.SaveRole(role)
			if err != nil {
				return fmt.Errorf("Error restoring role: %w", err)
			}
			fmt.Println("Restored role: ", role.Name)
		}

		fmt.Println("Restoring users...")
		for _, user := range backupData.Users {
			err := db.SaveUser(user.User, nil)
			if err != nil {
				return fmt.Errorf("Error restoring user: %w", err)
			}
			fmt.Println("Restored user: ", user.User.Username)

			// Restore user tokens
			fmt.Println("Restoring tokens for user: ", user.User.Username)
			for _, token := range user.Tokens {
				err = db.SaveToken(token)
				if err != nil {
					return fmt.Errorf("Error restoring token for user: %w", err)
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
					return fmt.Errorf("Error restoring space: %w", err)
				}
				fmt.Println("Restored space: ", space.Name)
			}
		}

		fmt.Println("Database restore completed successfully.")
		return nil
	},
}
