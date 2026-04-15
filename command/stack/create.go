package command_stack

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var CreateCmd = &cli.Command{
	Name:        "create",
	Usage:       "Create spaces from a stack definition",
	Description: "Create spaces from a stack definition, prefixed and grouped under a stack name.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "definition",
			Usage:    "Name of the stack definition to use",
			Required: true,
		},
		&cli.StringArg{
			Name:     "prefix",
			Usage:    "Prefix for space names (spaces are named prefix-key)",
			Required: true,
		},
		&cli.StringArg{
			Name:     "name",
			Usage:    "Stack name to group spaces under (defaults to prefix)",
			Required: false,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		defName := cmd.GetStringArg("definition")
		prefix := cmd.GetStringArg("prefix")
		stackName := cmd.GetStringArg("name")
		if stackName == "" {
			stackName = prefix
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			fmt.Println("Failed to create API client:", err)
			os.Exit(1)
		}

		def, err := client.GetStackDefinitionByName(ctx, defName)
		if err != nil {
			fmt.Println("Error looking up stack definition:", err)
			os.Exit(1)
		}
		if def == nil {
			fmt.Printf("Stack definition %q not found.\n", defName)
			os.Exit(1)
		}

		// Resolve template names to IDs for each component
		type createdSpace struct {
			key    string
			id     string
			space  *apiclient.StackDefSpace
		}
		spaces := make([]createdSpace, 0, len(def.Spaces))

		// Pass 1: Create all spaces
		for i := range def.Spaces {
			comp := &def.Spaces[i]
			spaceName := prefix + "-" + comp.Name

			templateId := comp.TemplateId

			customFields := make([]apiclient.CustomFieldValue, 0, len(comp.CustomFields))
			for _, cf := range comp.CustomFields {
				customFields = append(customFields, apiclient.CustomFieldValue{
					Name:  cf.Name,
					Value: cf.Value,
				})
			}

			spaceId, _, err := client.CreateSpace(ctx, &apiclient.SpaceRequest{
				Name:         spaceName,
				TemplateId:   templateId,
				Stack:        stackName,
				Description:  comp.Description,
				Shell:        comp.Shell,
				CustomFields: customFields,
			})
			if err != nil {
				fmt.Printf("Error creating space %q: %v\n", spaceName, err)
				// Attempt to clean up already-created spaces
				for _, s := range spaces {
					client.DeleteSpace(ctx, s.id)
				}
				os.Exit(1)
			}

			spaces = append(spaces, createdSpace{key: comp.Name, id: spaceId, space: comp})
			fmt.Printf("  Created space %q (%s)\n", spaceName, spaceId)
		}

		// Build key-to-ID map for dependency and port forward resolution
		keyToID := make(map[string]string)
		for _, s := range spaces {
			keyToID[s.key] = s.id
		}

		// Pass 2: Set dependencies
		for _, s := range spaces {
			if len(s.space.DependsOn) == 0 {
				continue
			}
			depIDs := make([]string, 0, len(s.space.DependsOn))
			for _, depKey := range s.space.DependsOn {
				if id, ok := keyToID[depKey]; ok {
					depIDs = append(depIDs, id)
				} else {
					fmt.Printf("  Warning: dependency %q not found for space %q\n", depKey, s.key)
				}
			}
			if len(depIDs) > 0 {
				spaceName := prefix + "-" + s.key
				_, err := client.UpdateSpace(ctx, s.id, &apiclient.SpaceRequest{
					Name:      spaceName,
					DependsOn: depIDs,
					Stack:     stackName,
				})
				if err != nil {
					fmt.Printf("  Warning: failed to set dependencies for %q: %v\n", spaceName, err)
				}
			}
		}

		// Pass 3: Apply port forwards
		for _, s := range spaces {
			if len(s.space.PortForwards) == 0 {
				continue
			}
			forwards := make([]apiclient.PortForwardRequest, 0, len(s.space.PortForwards))
			for _, pf := range s.space.PortForwards {
				targetID, ok := keyToID[pf.ToSpace]
				if !ok {
					fmt.Printf("  Warning: port forward target %q not found for space %q\n", pf.ToSpace, s.key)
					continue
				}
				forwards = append(forwards, apiclient.PortForwardRequest{
					LocalPort:  pf.LocalPort,
					Space:      targetID,
					RemotePort: pf.RemotePort,
					Persistent: true,
				})
			}
			if len(forwards) > 0 {
				_, _, err := client.ApplyPorts(ctx, s.id, &apiclient.PortApplyRequest{Forwards: forwards})
				if err != nil {
					fmt.Printf("  Warning: failed to apply port forwards for space %q: %v\n", s.key, err)
				}
			}
		}

		fmt.Printf("\nStack %q created from definition %q with %d space(s).\n", stackName, defName, len(spaces))
		fmt.Printf("Run 'knot stack start %s' to start all spaces.\n", stackName)
		return nil
	},
}
