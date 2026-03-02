package skills

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var showCmd = &cli.Command{
	Name:        "show",
	Usage:       "Show skill details",
	Description: "Show details of a specific skill.",
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

		fmt.Printf("Name: %s\n", skill.Name)
		fmt.Printf("Description: %s\n", skill.Description)
		fmt.Printf("Active: %t\n", skill.Active)
		fmt.Printf("Groups: %v\n", skill.Groups)
		fmt.Printf("Zones: %v\n", skill.Zones)
		fmt.Printf("\nContent:\n%s\n", skill.Content)
		return nil
	},
}
