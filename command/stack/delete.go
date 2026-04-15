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
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		user, err := client.WhoAmI(ctx)
		if err != nil {
			fmt.Println("Error getting user:", err)
			os.Exit(1)
		}

		spaces, _, err := client.GetSpaces(ctx, user.Id)
		if err != nil {
			fmt.Println("Error getting spaces:", err)
			os.Exit(1)
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

		for _, s := range stackSpaces {
			_, err := client.DeleteSpace(ctx, s.id)
			if err != nil {
				fmt.Printf("Error deleting space %q: %v\n", s.name, err)
			} else {
				fmt.Printf("  Deleted space %q\n", s.name)
			}
		}

		fmt.Printf("Stack %q deleted.\n", name)
		return nil
	},
}
