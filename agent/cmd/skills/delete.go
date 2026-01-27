package skills

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var deleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete a skill",
	Description: "Delete a skill by name.",
	MinArgs:     1,
	MaxArgs:     1,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		args := cmd.GetArgs()
		skill, err := client.GetSkill(ctx, args[0])
		if err != nil {
			return fmt.Errorf("error getting skill: %w", err)
		}

		err = client.DeleteSkill(ctx, skill.Id)
		if err != nil {
			return fmt.Errorf("error deleting skill: %w", err)
		}

		fmt.Printf("Skill %s deleted\n", args[0])
		return nil
	},
}
