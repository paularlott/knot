package command_stack

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

func stackSpaceStatus(space apiclient.SpaceInfo) string {
	if space.IsDeployed {
		if space.IsPending {
			return "Stopping"
		}
		return "Running"
	}
	if space.IsDeleting {
		return "Deleting"
	}
	if space.IsPending {
		return "Starting"
	}
	return "Stopped"
}

func stackSpaceHealth(space apiclient.SpaceInfo) string {
	if !space.IsDeployed || space.IsPending || space.IsDeleting {
		return "-"
	}
	if space.Healthy {
		return "Healthy"
	}
	return "Unhealthy"
}

var ListCmd = &cli.Command{
	Name:        "list",
	Usage:       "List stacks and their status",
	Description: "Lists all stacks for the logged in user and the status and health of their spaces.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		user, err := client.WhoAmI(context.Background())
		if err != nil {
			fmt.Println("Error getting user:", err)
			os.Exit(1)
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id, false)
		if err != nil {
			fmt.Println("Error getting spaces:", err)
			os.Exit(1)
		}

		// Group spaces by stack, preserving first-seen order.
		order := []string{}
		stacks := map[string][][]string{}
		for _, space := range spaces.Spaces {
			if space.Stack == "" {
				continue
			}
			if _, seen := stacks[space.Stack]; !seen {
				order = append(order, space.Stack)
				stacks[space.Stack] = [][]string{}
			}

			stacks[space.Stack] = append(stacks[space.Stack], []string{
				fmt.Sprintf("%s (%s)", space.Name, stackSpaceStatus(space)),
				stackSpaceHealth(space),
			})
		}

		if len(order) == 0 {
			fmt.Println("No stacks found.")
			return nil
		}

		data := [][]string{{"Stack", "Spaces", "Health"}}
		for _, name := range order {
			first := true
			for _, entry := range stacks[name] {
				if first {
					data = append(data, []string{name, entry[0], entry[1]})
					first = false
				} else {
					data = append(data, []string{"", entry[0], entry[1]})
				}
			}
		}

		util.PrintTable(data)
		return nil
	},
}
