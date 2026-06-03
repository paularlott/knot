package command_scripts

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var writeCmd = &cli.Command{
	Name:        "write",
	Usage:       "Write or update a script",
	Description: "Write a script from a file or stdin. Usage: knot script write <name> [file]",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the script",
			Required: true,
		},
		&cli.StringArg{
			Name:     "file",
			Usage:    "Path to script file (reads from stdin if not given)",
			Required: false,
		},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:    "create",
			Aliases: []string{"c"},
			Usage:   "Create the script if it does not exist.",
		},
		&cli.StringFlag{
			Name:    "description",
			Aliases: []string{"d"},
			Usage:   "Description of the script.",
		},
		&cli.BoolFlag{
			Name:         "active",
			Usage:        "Set the script as active.",
			DefaultValue: true,
		},
	},
	MaxArgs: cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		global := cmd.GetBool("global")
		create := cmd.GetBool("create")
		scriptName := cmd.GetStringArg("name")
		filePath := cmd.GetStringArg("file")

		var content string
		if filePath != "" {
			data, err := os.ReadFile(filePath)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			content = string(data)
		} else {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %w", err)
			}
			content = string(data)
		}

		userId := "current"
		if global {
			userId = ""
		}

		existing, err := resolveScript(ctx, cmd, client, scriptName)
		if err != nil {
			if !create {
				return fmt.Errorf("script %s not found, use --create to create it", scriptName)
			}

			req := apiclient.ScriptCreateRequest{
				UserId:      userId,
				Name:        scriptName,
				Description: cmd.GetString("description"),
				Content:     content,
				Active:      cmd.GetBool("active"),
			}

			resp, err := client.CreateScript(ctx, req)
			if err != nil {
				return fmt.Errorf("error creating script: %w", err)
			}
			fmt.Printf("Script %s created (id: %s)\n", scriptName, resp.Id)
			return nil
		}

		req := apiclient.ScriptUpdateRequest{
			Name:               scriptName,
			Description:        cmd.GetString("description"),
			Content:            content,
			Active:             cmd.GetBool("active"),
			ScriptType:         existing.ScriptType,
			Groups:             existing.Groups,
			Zones:              existing.Zones,
			MCPInputSchemaToml: existing.MCPInputSchemaToml,
			MCPKeywords:        existing.MCPKeywords,
			Discoverable:       existing.Discoverable,
		}
		if req.Description == "" {
			req.Description = existing.Description
		}

		err = client.UpdateScript(ctx, existing.Id, req)
		if err != nil {
			return fmt.Errorf("error updating script: %w", err)
		}
		fmt.Printf("Script %s updated\n", scriptName)
		return nil
	},
}
