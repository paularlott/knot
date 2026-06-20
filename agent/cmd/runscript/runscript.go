package runscript

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/agentlink"
	knotmethods "github.com/paularlott/knot/internal/methods"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling/object"
)

var RunScriptCmd = &cli.Command{
	Name:        "run-script",
	Usage:       "Run a script in this space",
	Description: "Execute a named script or local script file in this space. Usage: knot run-script <script-name-or-file> [args...]",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "script",
			Usage:    "Name of script or path to .py file",
			Required: true,
		},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "no-fail",
			Usage: "Exit successfully if the named script does not exist.",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)

		scriptArg := cmd.GetStringArg("script")
		args := cmd.GetArgs()
		argv := append([]string{scriptArg}, args...)

		var content string
		if _, err := os.Stat(scriptArg); err == nil {
			data, err := os.ReadFile(scriptArg)
			if err != nil {
				return fmt.Errorf("failed to read script file: %w", err)
			}
			content = string(data)
		} else {
			fetched, err := client.GetScriptByName(ctx, scriptArg)
			if err != nil {
				if cmd.GetBool("no-fail") && errors.Is(err, apiclient.ErrScriptNotFound) {
					return nil
				}
				return fmt.Errorf("failed to fetch script: %w", err)
			}
			content = fetched
		}

		var userId string
		user, err := client.WhoAmI(ctx)
		if err == nil {
			userId = user.Id
		}

		// knot run-script executes in the CLI process. Wire the methods
		// registrar to the agentlink command socket so server.register()
		// publishes to the daemon (same path as `knot methods register`).
		knotscriptling.SetMethodsRegistrar(func(reg *knotmethods.Registration) error {
			var resp agentlink.RegisterMethodsResponse
			if err := agentlink.SendWithResponseMsg(agentlink.CommandRegisterMethods, agentlink.RegisterMethodsRequest{Registration: *reg}, &resp); err != nil {
				return err
			}
			if !resp.Success {
				return errors.New(resp.Error)
			}
			return nil
		})

		env, err := service.NewRemoteStreamingScriptlingEnv(argv, client, userId, nil, os.Stdout, os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to create script environment: %w", err)
		}

		result, err := env.EvalWithContext(ctx, content)

		if ex, ok := object.AsException(result); ok && ex.IsSystemExit() {
			os.Exit(ex.GetExitCode())
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return nil
	},
}
