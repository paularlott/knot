package skills

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var createCmd = &cli.Command{
	Name:        "create",
	Usage:       "Create a skill",
	Description: "Create a skill from a file.",
	MinArgs:     1,
	MaxArgs:     1,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "global",
			Aliases: []string{"g"},
			Usage:   "Create as global skill",
		},
		&cli.StringSliceFlag{
			Name:    "group",
			Usage:   "Group IDs (can be specified multiple times)",
		},
		&cli.StringSliceFlag{
			Name:    "zone",
			Usage:   "Zone names (can be specified multiple times)",
		},
		&cli.BoolFlag{
			Name:         "active",
			Usage:        "Set skill as active",
			DefaultValue: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		args := cmd.GetArgs()
		content, err := os.ReadFile(args[0])
		if err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}

		userId := "current"
		if cmd.GetBool("global") {
			userId = ""
		}

		req := apiclient.SkillCreateRequest{
			UserId:  userId,
			Content: string(content),
			Groups:  cmd.GetStringSlice("group"),
			Zones:   cmd.GetStringSlice("zone"),
			Active:  cmd.GetBool("active"),
		}

		resp, err := client.CreateSkill(ctx, &req)
		if err != nil {
			return fmt.Errorf("error creating skill: %w", err)
		}

		fmt.Printf("Skill created with ID: %s\n", resp.Id)
		return nil
	},
}
