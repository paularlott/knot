package command_stack

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var DeleteCmd = &cli.Command{
	Name:        "delete",
	Usage:       "Delete a stack and all its spaces",
	Description: "Delete all spaces belonging to the named stack.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the stack to delete",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Skip confirmation prompt",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		name := cmd.GetStringArg("name")

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}

		user, err := client.WhoAmI(ctx)
		if err != nil {
			return fmt.Errorf("couldn't get the current user: %w", err)
		}

		spaces, _, err := client.GetSpaces(ctx, user.Id, false)
		if err != nil {
			return fmt.Errorf("couldn't list spaces: %w", err)
		}

		// Collect spaces belonging to this stack
		type stackSpace struct {
			id   string
			name string
		}
		var stackSpaces []stackSpace
		for _, s := range spaces.Spaces {
			if s.Stack == name {
				stackSpaces = append(stackSpaces, stackSpace{id: s.Id, name: s.Name})
			}
		}

		if len(stackSpaces) == 0 {
			fmt.Printf("No spaces found for stack %q.\n", name)
			return nil
		}

		if !cmd.GetBool("yes") {
			fmt.Printf("Delete stack %q and its %d space(s)?\n", name, len(stackSpaces))
			for _, s := range stackSpaces {
				fmt.Printf("  - %s\n", s.name)
			}
			fmt.Print("\n[y/N] ")
			reader := bufio.NewReader(os.Stdin)
			answer, _ := reader.ReadString('\n')
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		_, err = client.DeleteStack(ctx, name)
		if err != nil {
			return fmt.Errorf("couldn't delete stack %q: %w", name, err)
		}

		fmt.Printf("Stack %q deleting.\n", name)
		return nil
	},
}
