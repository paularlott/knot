package command_stack

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var ListDefsCmd = &cli.Command{
	Name:        "list-defs",
	Usage:       "List stack definitions",
	Description: "List all stack definitions with details.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "details",
			Usage: "Show space details for each definition",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		list, _, err := client.GetStackDefinitions(ctx)
		if err != nil {
			fmt.Println("Error listing stack definitions:", err)
			os.Exit(1)
		}

		if list.Count == 0 {
			fmt.Println("No stack definitions found.")
			return nil
		}

		details := cmd.GetBool("details")

		if !details {
			data := [][]string{{"Name", "Scope", "Zones", "Spaces", "Active", "Description"}}
			for _, d := range list.Definitions {
				zones := "all"
				if len(d.Zones) > 0 {
					zones = strings.Join(d.Zones, ",")
				}
				active := "yes"
				if !d.Active {
					active = "no"
				}
				data = append(data, []string{
					d.Name,
					d.Scope,
					zones,
					fmt.Sprintf("%d", len(d.Spaces)),
					active,
					d.Description,
				})
			}
			util.PrintTable(data)
			return nil
		}

		// Detailed view
		for _, d := range list.Definitions {
			zones := "all zones"
			if len(d.Zones) > 0 {
				zones = strings.Join(d.Zones, ", ")
			}
			fmt.Printf("%s (%s, %s)", d.Name, d.Scope, zones)
			if d.Description != "" {
				fmt.Printf(" — %s", d.Description)
			}
			fmt.Println()
			for _, s := range d.Spaces {
				depends := "(none)"
				if len(s.DependsOn) > 0 {
					depends = strings.Join(s.DependsOn, ", ")
				}
				forwards := "(none)"
				if len(s.PortForwards) > 0 {
					parts := make([]string, 0, len(s.PortForwards))
					for _, pf := range s.PortForwards {
						parts = append(parts, fmt.Sprintf("%s:%d → %s:%d", s.Name, pf.LocalPort, pf.ToSpace, pf.RemotePort))
					}
					forwards = strings.Join(parts, ", ")
				}
				fmt.Printf("  %-6s %-20s depends: [%s]  forwards: %s\n", s.Name, s.TemplateId, depends, forwards)
			}
		}

		return nil
	},
}
