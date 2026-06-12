package command_scripts

import (
	"context"
	"fmt"
	"os"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var readCmd = &cli.Command{
	Name:        "read",
	Usage:       "Read a script",
	Description: "Read a script's content. Use --info to show metadata instead.",
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "name",
			Usage:    "Name of the script",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "info",
			Usage: "Show script metadata instead of content.",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		script, err := resolveScript(ctx, cmd, client, cmd.GetStringArg("name"))
		if err != nil {
			return err
		}

		if cmd.GetBool("info") {
			fmt.Printf("Name: %s\n", script.Name)
			if script.UserId != "" {
				fmt.Printf("Scope: user\n")
			} else {
				fmt.Printf("Scope: global\n")
			}
			fmt.Printf("Description: %s\n", script.Description)
			fmt.Printf("Active: %t\n", script.Active)
			fmt.Printf("Type: %s\n", script.ScriptType)
			if len(script.Groups) > 0 {
				fmt.Printf("Groups: %v\n", script.Groups)
			}
			if script.ScriptType == "tool" {
				fmt.Printf("MCP Keywords: %v\n", script.MCPKeywords)
			}
			return nil
		}

		fmt.Fprint(os.Stdout, script.Content)
		return nil
	},
}
