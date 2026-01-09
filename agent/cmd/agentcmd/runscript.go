package agentcmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/service"
)

var RunScriptCmd = &cli.Command{
	Name:        "run-script",
	Usage:       "Run a scriptling script",
	Description: "Execute a scriptling script from disk or by name from the server.",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "script",
			Usage:    "Script name or file path",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "Server alias to use for fetching scripts.",
			DefaultValue: "default",
		},
		&cli.BoolFlag{
			Name:    "tls-skip-verify",
			Usage:   "Skip TLS verification.",
			EnvVars: []string{"KNOT_TLS_SKIP_VERIFY"},
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		script := cmd.GetStringArg("script")
		args := cmd.GetArgs()

		// Get client using unified approach (works in both desktop and agent contexts)
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)

		// Get user info
		user, err := client.WhoAmI(ctx)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		var scriptContent string

		// Check if file exists locally
		if _, err := os.Stat(script); err == nil {
			content, err := os.ReadFile(script)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			scriptContent = string(content)
		} else {
			scriptContent, err = client.GetScriptByName(ctx, script)
			if err != nil {
				return fmt.Errorf("failed to get script: %w", err)
			}
		}

		output, err := service.RunScript(ctx, scriptContent, args, client, user.Id)
		if err != nil {
			return fmt.Errorf("script execution failed: %w", err)
		}

		if output != "" {
			fmt.Println(output)
		}
		return nil
	},
}
