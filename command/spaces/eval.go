package command_spaces

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
)

var EvalCmd = &cli.Command{
	Name:        "eval",
	Usage:       "Evaluate inline Scriptling code in a space",
	Description: "Execute Scriptling source directly in a space without storing a named script.\n\nUsage: space eval <space-name> <code|-> [args...]\n\nPass the code as a quoted argument, or use '-' to read it from stdin. Everything after the code positional is forwarded to the script as argv.\n\nExamples:\n  knot space eval web \"print('hello')\"\n  knot space eval web \"print(sys.argv)\" one two\n  echo 'print(1)' | knot space eval web -\n  cat tuned.py | knot space eval web - --flag",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space-name",
			Usage:    "Name of the space",
			Required: true,
		},
		&cli.StringArg{
			Name:     "code",
			Usage:    "Scriptling source to evaluate, or '-' to read from stdin",
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

		code := cmd.GetStringArg("code")
		if code == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read code from stdin: %w", err)
			}
			code = string(data)
		}
		if code == "" {
			return fmt.Errorf("no code to evaluate (pass code as an argument or pipe via '-')")
		}

		// Forward trailing positionals to the script as argv. argv[0] is the
		// invocation name, matching run-script's convention.
		argv := append([]string{"eval"}, cmd.GetArgs()...)

		exitCode, err := client.ExecuteScriptContentStream(ctx, space.SpaceId, code, argv)
		if err != nil {
			return fmt.Errorf("error executing code: %w", err)
		}
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return nil
	},
}
