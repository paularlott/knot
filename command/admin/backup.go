package commands_admin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/cli"
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
	AuditLogs    []*model.AuditLogEntry
}

var BackupCmd = &cli.Command{
	Name:        "backup",
	Usage:       "Backup to File",
	Description: "Backup the database to a backup file.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "backupfile",
			Usage:    "The name of the backup file",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:       "templates",
			Aliases:    []string{"t"},
			Usage:      "Backup templates",
			ConfigPath: []string{"backup.templates"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TEMPLATES"},
		},
		&cli.BoolFlag{
			Name:       "template-vars",
			Aliases:    []string{"v"},
			Usage:      "Backup template variables",
			ConfigPath: []string{"backup.template_vars"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TEMPLATE_VARS"},
		},
		&cli.BoolFlag{
			Name:       "volumes",
			Aliases:    []string{"l"},
			Usage:      "Backup volumes",
			ConfigPath: []string{"backup.volumes"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_VOLUMES"},
		},
		&cli.BoolFlag{
			Name:       "groups",
			Aliases:    []string{"g"},
			Usage:      "Backup groups",
			ConfigPath: []string{"backup.groups"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_GROUPS"},
		},
		&cli.BoolFlag{
			Name:       "roles",
			Aliases:    []string{"r"},
			Usage:      "Backup roles",
			ConfigPath: []string{"backup.roles"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ROLES"},
		},
		&cli.BoolFlag{
			Name:       "spaces",
			Aliases:    []string{"s"},
			Usage:      "Backup user spaces",
			ConfigPath: []string{"backup.spaces"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_SPACES"},
		},
		&cli.BoolFlag{
			Name:       "users",
			Aliases:    []string{"u"},
			Usage:      "Backup users",
			ConfigPath: []string{"backup.users"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_USERS"},
		},
		&cli.BoolFlag{
			Name:       "tokens",
			Aliases:    []string{"k"},
			Usage:      "Backup user tokens",
			ConfigPath: []string{"backup.tokens"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TOKENS"},
		},
		&cli.BoolFlag{
			Name:       "cfg-values",
			Aliases:    []string{"o"},
			Usage:      "Backup configuration values",
			ConfigPath: []string{"backup.cfg_values"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_CFG_VALUES"},
		},
		&cli.BoolFlag{
			Name:       "audit-logs",
			Usage:      "Backup audit logs",
			ConfigPath: []string{"backup.audit_logs"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_AUDIT_LOGS"},
		},
		&cli.BoolFlag{
			Name:         "all",
			Aliases:      []string{"a"},
			Usage:        "Backup everything",
			ConfigPath:   []string{"backup.all"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ALL"},
			DefaultValue: true,
		},
		&cli.StringFlag{
			Name:       "limit-user",
			Usage:      "Limit the backup to a specific user by username.",
			ConfigPath: []string{"backup.limit_user"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_LIMIT_USER"},
		},
		&cli.StringFlag{
			Name:       "limit-template",
			Usage:      "Limit the backup to a specific template by name.",
			ConfigPath: []string{"backup.limit_template"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_LIMIT_TEMPLATE"},
		},
		&cli.StringFlag{
			Name:       "encrypt-key",
			Aliases:    []string{"e"},
			Usage:      "Encrypt the backup file with the given key. The key must be 32 bytes long.",
			ConfigPath: []string{"backup.encrypt_key"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ENCRYPT_KEY"},
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		outputFile := cmd.GetStringArg("backupfile")
		fmt.Println("Backing up database to file: ", outputFile)

		backupTemplates := cmd.GetBool("templates")
		backupVars := cmd.GetBool("template-vars")
		backupVolumes := cmd.GetBool("volumes")
		backupGroups := cmd.GetBool("groups")
		backupRoles := cmd.GetBool("roles")
		backupUsers := cmd.GetBool("users")
		backupSpaces := cmd.GetBool("spaces")
		backupTokens := cmd.GetBool("tokens")
		backupCfgValues := cmd.GetBool("cfg-values")
		backupAuditLogs := cmd.GetBool("audit-logs")
		backupAll := cmd.GetBool("all")

		// If any specific backup flags are set, do not use the "all" flag
		if backupTemplates || backupVars || backupVolumes || backupGroups || backupRoles || backupUsers || backupSpaces || backupTokens || backupCfgValues || backupAuditLogs {
			backupAll = false
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
			backupAuditLogs = true
		}

		limitUser := cmd.GetString("limit-user")
		limitTemplate := cmd.GetString("limit-template")

		key := cmd.GetString("encrypt-key")
		if key != "" && len(key) != 32 {
			return fmt.Errorf("Error: Encrypt key must be 32 bytes long.")
		}

		db := database.GetInstance()
		backupData := backupData{}

		if backupAuditLogs {
			fmt.Println("Backing up audit logs...")
			auditLogs, err := db.GetAuditLogs(0, 0)
			if err != nil {
				return fmt.Errorf("Error getting audit logs: %w", err)
			}
			backupData.AuditLogs = make([]*model.AuditLogEntry, len(auditLogs))
			copy(backupData.AuditLogs, auditLogs)
		}

		if backupCfgValues {
			fmt.Println("Backing up configuration values...")
			cfgValues, err := db.GetCfgValues()
			if err != nil {
				return fmt.Errorf("Error getting configuration values: %w", err)
			}
			backupData.CfgValues = make([]*model.CfgValue, len(cfgValues))
			copy(backupData.CfgValues, cfgValues)
		}

		if backupTemplates {
			fmt.Println("Backing up templates...")
			templates, err := db.GetTemplates()
			if err != nil {
				return fmt.Errorf("Error getting templates: %w", err)
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
			variables, err := db.GetTemplateVars()
			if err != nil {
				return fmt.Errorf("Error getting template variables: %w", err)
			}
			backupData.TemplateVars = make([]*model.TemplateVar, len(variables))
			for i, v := range variables {
				backupData.TemplateVars[i] = v
			}
		}

		if backupVolumes {
			fmt.Println("Backing up volumes...")
			volumes, err := db.GetVolumes()
			if err != nil {
				return fmt.Errorf("Error getting volumes: %w", err)
			}
			backupData.Volumes = make([]*model.Volume, len(volumes))
			copy(backupData.Volumes, volumes)
		}

		if backupGroups {
			fmt.Println("Backing up groups...")
			groups, err := db.GetGroups()
			if err != nil {
				return fmt.Errorf("Error getting groups: %w", err)
			}
			backupData.Groups = make([]*model.Group, len(groups))
			copy(backupData.Groups, groups)
		}

		if backupRoles {
			fmt.Println("Backing up roles...")
			roles, err := db.GetRoles()
			if err != nil {
				return fmt.Errorf("Error getting roles: %w", err)
			}
			backupData.Roles = make([]*model.Role, len(roles))
			copy(backupData.Roles, roles)
		}

		if backupUsers {
			fmt.Println("Backing up users...")
			users, err := db.GetUsers()
			if err != nil {
				return fmt.Errorf("Error getting users: %w", err)
			}
			backupData.Users = make([]backupUser, 0, len(users))
			for _, u := range users {
				if limitUser != "" && u.Username != limitUser {
					continue
				}
				bu := backupUser{
					User: u,
				}
				if backupTokens {
					tokens, err := db.GetTokensForUser(u.Id)
					if err != nil {
						return fmt.Errorf("Error getting tokens for user: %w", err)
					}
					bu.Tokens = make([]*model.Token, len(tokens))
					copy(bu.Tokens, tokens)
				}
				if backupSpaces {
					spaces, err := db.GetSpacesForUser(u.Id)
					if err != nil {
						return fmt.Errorf("Error getting spaces: %w", err)
					}
					bu.Spaces = make([]*model.Space, len(spaces))
					for j, s := range spaces {
						space, err := db.GetSpace(s.Id)
						if err != nil {
							return fmt.Errorf("Error getting space: %w", err)
						}
						bu.Spaces[j] = space
					}
				}
				backupData.Users = append(backupData.Users, bu)
			}
			backupData.Users = slices.Clip(backupData.Users)
		}

		data, err := json.Marshal(backupData)
		if err != nil {
			return fmt.Errorf("Error marshalling backup data: %w", err)
		}

		if key != "" {
			data = []byte(crypt.Encrypt(key, string(data)))
		}

		err = os.WriteFile(outputFile, data, 0644)
		if err != nil {
			return fmt.Errorf("Error writing backup file: %w", err)
		}

		fmt.Println("Database backup completed successfully.")
		return nil
	},
}
