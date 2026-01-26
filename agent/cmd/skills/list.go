package skills

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var listCmd = &cli.Command{
	Name:    "list",
	Usage:   "List skills",
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		skills, err := client.GetSkills(ctx)
		if err != nil {
			return fmt.Errorf("error getting skills: %w", err)
		}

		if skills.Count == 0 {
			fmt.Println("No skills found")
			return nil
		}

		// Separate skills into user and global
		var userSkills, globalSkills []apiclient.SkillInfo
		for _, skill := range skills.Skills {
			if skill.UserId != "" {
				userSkills = append(userSkills, skill)
			} else {
				globalSkills = append(globalSkills, skill)
			}
		}

		// Print user skills first (if any)
		if len(userSkills) > 0 {
			fmt.Println("\nUser Skills:")
			table := [][]string{
				{"NAME", "DESCRIPTION", "ACTIVE"},
			}
			for _, skill := range userSkills {
				active := "No"
				if skill.Active {
					active = "Yes"
				}
				table = append(table, []string{
					skill.Name,
					skill.Description,
					active,
				})
			}
			util.PrintTable(table)
		}

		// Print global skills (if any)
		if len(globalSkills) > 0 {
			fmt.Println("\nGlobal Skills:")
			table := [][]string{
				{"NAME", "DESCRIPTION", "ACTIVE"},
			}
			for _, skill := range globalSkills {
				active := "No"
				if skill.Active {
					active = "Yes"
				}
				table = append(table, []string{
					skill.Name,
					skill.Description,
					active,
				})
			}
			util.PrintTable(table)
		}

		return nil
	},
}
