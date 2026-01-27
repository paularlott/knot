package skills

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var updateCmd = &cli.Command{
	Name:        "update",
	Usage:       "Update a skill",
	Description: "Update a skill by name.",
	MinArgs:     1,
	MaxArgs:     2,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "group",
			Usage: "Group IDs (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:  "zone",
			Usage: "Zone names (can be specified multiple times)",
		},
		&cli.BoolFlag{
			Name:  "active",
			Usage: "Set skill as active",
		},
		&cli.BoolFlag{
			Name:  "inactive",
			Usage: "Set skill as inactive",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		args := cmd.GetArgs()
		skillName := args[0]

		skill, err := client.GetSkill(ctx, skillName)
		if err != nil {
			return fmt.Errorf("error getting skill: %w", err)
		}

		req := apiclient.SkillUpdateRequest{
			Content: skill.Content,
			Groups:  skill.Groups,
			Zones:   skill.Zones,
			Active:  skill.Active,
		}

		// Update content if file provided
		if len(args) > 1 {
			content, err := os.ReadFile(args[1])
			if err != nil {
				return fmt.Errorf("error reading file: %w", err)
			}
			req.Content = string(content)
		}

		// Update groups if provided
		groups := cmd.GetStringSlice("group")
		if len(groups) > 0 {
			req.Groups = groups
		}

		// Update zones if provided
		zones := cmd.GetStringSlice("zone")
		if len(zones) > 0 {
			req.Zones = zones
		}

		// Update active status
		if cmd.GetBool("active") {
			req.Active = true
		} else if cmd.GetBool("inactive") {
			req.Active = false
		}

		err = client.UpdateSkill(ctx, skill.Id, &req)
		if err != nil {
			return fmt.Errorf("error updating skill: %w", err)
		}

		fmt.Printf("Skill %s updated\n", skillName)
		return nil
	},
}
