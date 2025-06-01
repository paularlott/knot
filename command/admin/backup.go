package commands_admin

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/spf13/cobra"
)

type backupUser struct {
	User   *model.User
	Tokens []*model.Token
	Spaces []*model.Space
}

type backupData struct {
	Templates    []*model.Template
	TemplateVars []*model.TemplateVar
	Volumes      []*model.Volume
	Groups       []*model.Group
	Roles        []*model.Role
	Users        []backupUser
	CfgValues    []*model.CfgValue
}

func init() {
	backupCmd.Flags().BoolP("templates", "t", false, "Backup templates")
	backupCmd.Flags().BoolP("template-vars", "v", false, "Backup template variables")
	backupCmd.Flags().BoolP("volumes", "l", false, "Backup volumes")
	backupCmd.Flags().BoolP("groups", "g", false, "Backup groups")
	backupCmd.Flags().BoolP("roles", "r", false, "Backup roles")
	backupCmd.Flags().BoolP("spaces", "s", false, "Backup user spaces")
	backupCmd.Flags().BoolP("users", "u", false, "Backup users")
	backupCmd.Flags().BoolP("tokens", "k", false, "Backup user tokens")
	backupCmd.Flags().BoolP("cfg-values", "o", false, "Backup configuration values")
	backupCmd.Flags().BoolP("all", "a", true, "Backup everything")
	backupCmd.Flags().StringP("limit-user", "", "", "Limit the backup to a specific user by username.")
	backupCmd.Flags().StringP("limit-template", "", "", "Limit the backup to a specific template by name.")
	backupCmd.Flags().StringP("encrypt-key", "e", "", "Encrypt the backup file with the given key. The key must be 32 bytes long.")
}

var backupCmd = &cobra.Command{
	Use:   "backup <backupfile> [flags]",
	Short: "Backup the database",
	Long:  `Backup the database a backup file.`,
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFile := args[0]
		fmt.Println("Backing up database to file: ", outputFile)

		backupTemplates, _ := cmd.Flags().GetBool("templates")
		backupVars, _ := cmd.Flags().GetBool("template-vars")
		backupVolumes, _ := cmd.Flags().GetBool("volumes")
		backupGroups, _ := cmd.Flags().GetBool("groups")
		backupRoles, _ := cmd.Flags().GetBool("roles")
		backupUsers, _ := cmd.Flags().GetBool("users")
		backupSpaces, _ := cmd.Flags().GetBool("spaces")
		backupTokens, _ := cmd.Flags().GetBool("tokens")
		backupCfgValues, _ := cmd.Flags().GetBool("cfg-values")
		backupAll, _ := cmd.Flags().GetBool("all")

		if backupTemplates || backupVars || backupVolumes || backupGroups || backupRoles || backupUsers || backupSpaces || backupTokens || backupCfgValues {
			backupAll = false // If any specific backup flags are set, do not use the "all" flag
		}

		if backupAll {
			backupTemplates = true
			backupVars = true
			backupVolumes = true
			backupGroups = true
			backupRoles = true
			backupUsers = true
			backupSpaces = true
			backupTokens = true
			backupCfgValues = true
		}

		limitUser, _ := cmd.Flags().GetString("limit-user")
		limitTemplate, _ := cmd.Flags().GetString("limit-template")

		key, _ := cmd.Flags().GetString("encrypt-key")
		if key != "" && len(key) != 32 {
			fmt.Println("Error: Encrypt key must be 32 bytes long.")
			os.Exit(1)
		}

		db := database.GetInstance()

		backupData := backupData{}

		if backupCfgValues {
			fmt.Println("Backing up configuration values...")

			// Get the list of configuration values
			cfgValues, err := db.GetCfgValues()
			if err != nil {
				fmt.Println("Error getting configuration values: ", err)
				os.Exit(1)
			}
			backupData.CfgValues = make([]*model.CfgValue, len(cfgValues))
			copy(backupData.CfgValues, cfgValues)
		}

		if backupTemplates {
			fmt.Println("Backing up templates...")

			// Get the list of templates
			templates, err := db.GetTemplates()
			if err != nil {
				fmt.Println("Error getting templates: ", err)
				os.Exit(1)
			}
			backupData.Templates = make([]*model.Template, 0, len(templates))
			for _, t := range templates {
				if limitTemplate == "" || t.Name == limitTemplate {
					backupData.Templates = append(backupData.Templates, t)
				}
			}
			backupData.Templates = slices.Clip(backupData.Templates)
		}

		if backupVars {
			fmt.Println("Backing up template variables...")

			// Get the list of template variables
			variables, err := db.GetTemplateVars()
			if err != nil {
				fmt.Println("Error getting template variables: ", err)
				os.Exit(1)
			}
			backupData.TemplateVars = make([]*model.TemplateVar, len(variables))
			for i, v := range variables {
				backupData.TemplateVars[i] = v
			}
		}

		if backupVolumes {
			fmt.Println("Backing up volumes...")

			// Get the list of volumes
			volumes, err := db.GetVolumes()
			if err != nil {
				fmt.Println("Error getting volumes: ", err)
				os.Exit(1)
			}
			backupData.Volumes = make([]*model.Volume, len(volumes))
			copy(backupData.Volumes, volumes)
		}

		if backupGroups {
			fmt.Println("Backing up groups...")

			// Get the list of groups
			groups, err := db.GetGroups()
			if err != nil {
				fmt.Println("Error getting groups: ", err)
				os.Exit(1)
			}
			backupData.Groups = make([]*model.Group, len(groups))
			copy(backupData.Groups, groups)
		}

		if backupRoles {
			fmt.Println("Backing up roles...")

			// Get the list of roles
			roles, err := db.GetRoles()
			if err != nil {
				fmt.Println("Error getting roles: ", err)
				os.Exit(1)
			}
			backupData.Roles = make([]*model.Role, len(roles))
			copy(backupData.Roles, roles)
		}

		if backupUsers {
			fmt.Println("Backing up users...")

			// Get the list of users and their tokens
			users, err := db.GetUsers()
			if err != nil {
				fmt.Println("Error getting users: ", err)
				os.Exit(1)
			}
			backupData.Users = make([]backupUser, 0, len(users))
			for _, u := range users {
				if limitUser != "" && u.Username != limitUser {
					continue // Skip users that do not match the limit
				}

				bu := backupUser{
					User: u,
				}

				if backupTokens {
					tokens, err := db.GetTokensForUser(u.Id)
					if err != nil {
						fmt.Println("Error getting tokens for user: ", err)
						os.Exit(1)
					}
					bu.Tokens = make([]*model.Token, len(tokens))
					copy(bu.Tokens, tokens)
				}

				if backupSpaces {
					// Get the list of spaces
					spaces, err := db.GetSpacesForUser(u.Id)
					if err != nil {
						fmt.Println("Error getting spaces: ", err)
						os.Exit(1)
					}
					bu.Spaces = make([]*model.Space, len(spaces))
					for j, s := range spaces {
						// Get the space again so that we get the alt names
						space, err := db.GetSpace(s.Id)
						if err != nil {
							fmt.Println("Error getting space: ", err)
							os.Exit(1)
						}

						bu.Spaces[j] = space
					}
				}

				backupData.Users = append(backupData.Users, bu)
			}

			backupData.Users = slices.Clip(backupData.Users)
		}

		// Marshal the backup data to json format
		data, err := json.Marshal(backupData)
		if err != nil {
			fmt.Println("Error marshalling backup data: ", err)
			os.Exit(1)
		}

		// If have an encryption key then encrypt the data
		if key != "" {
			data = []byte(crypt.Encrypt(key, string(data)))
		}

		// Write the backup data to the output file
		err = os.WriteFile(outputFile, data, 0644)
		if err != nil {
			fmt.Println("Error writing backup file: ", err)
			os.Exit(1)
		}

		fmt.Println("Database backup completed successfully.")
	},
}
