package command_spaces

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var RunScriptCmd = &cli.Command{
	Name:        "run-script",
	Usage:       "Run a script in a space",
	Description: "Execute a named script or local script file in a space. Usage: space run-script <space-name> <script-name-or-file> [args...]",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space-name",
			Usage:    "Name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "script",
			Usage:    "Name of script or path to .py file",
			Required: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)

		space, err := client.GetSpaceByName(ctx, cmd.GetStringArg("space-name"))
		if err != nil {
			return fmt.Errorf("error getting space: %w", err)
		}

		scriptArg := cmd.GetStringArg("script")
		args := cmd.GetArgs()

		// Prepend script name to argv (sys.argv[0] should be the script name)
		argv := append([]string{scriptArg}, args...)

		// Check if it's a file
		if _, err := os.Stat(scriptArg); err == nil {
			// It's a file - read and execute content via streaming
			content, err := os.ReadFile(scriptArg)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			exitCode, err := client.ExecuteScriptContentStream(ctx, space.SpaceId, string(content), argv)
			if err != nil {
				return fmt.Errorf("error executing script: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
		} else {
			// It's a named script - use streaming
			exitCode, err := client.ExecuteScriptStream(ctx, space.SpaceId, scriptArg, argv)
			if err != nil {
				return fmt.Errorf("error executing script: %w", err)
			}
			if exitCode != 0 {
				os.Exit(exitCode)
			}
		}
		return nil
	},
}
