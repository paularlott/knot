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
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling/object"
)

var RunScriptCmd = &cli.Command{
	Name:        "run-script",
	Usage:       "Run a script in this space",
	Description: "Execute a named script or local script file in this space, or start an interactive REPL. Usage: knot run-script <script-name-or-file> [args...] / knot run-script --interactive",
	MaxArgs:     cli.UnlimitedArgs,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "script",
			Usage:    "Name of script or path to .py file (omit with --interactive)",
			Required: false,
		},
	},
	Flags: append([]cli.Flag{
		&cli.BoolFlag{
			Name:     "interactive",
			Aliases:  []string{"i"},
			Usage:    "Start an interactive REPL in this space.",
		},
		&cli.BoolFlag{
			Name:  "no-fail",
			Usage: "Exit successfully if the named script does not exist.",
		},
	}, serveFlags...),
	Run: func(ctx context.Context, cmd *cli.Command) error {
		scriptArg := cmd.GetStringArg("script")
		args := cmd.GetArgs()
		argv := append([]string{scriptArg}, args...)

		_, statErr := os.Stat(scriptArg)
		localFile := statErr == nil

		// Server modes (json-rpc / http / mcp) run the script through the
		// scriptling server runtime instead of evaluating it once. Acquire a
		// client when the agent is connected so served handlers get knot.* libs;
		// a local file can still serve standalone (without those libs).
		if serveRequested(cmd) {
			var serveClient *apiclient.ApiClient
			var serveUserId string
			if agentlink.IsAgentRunning() {
				if c, err := cmdutil.GetClient(cmd); err == nil {
					c.SetTimeout(5 * time.Minute)
					serveClient = c
					if u, err := c.WhoAmI(ctx); err == nil {
						serveUserId = u.Id
					}
				}
			}

			scriptFile := scriptArg
			if !localFile {
				if serveClient == nil {
					return fmt.Errorf("cannot serve named script %q: no server connection", scriptArg)
				}
				fetched, err := serveClient.GetScriptByName(ctx, scriptArg)
				if err != nil {
					if cmd.GetBool("no-fail") && errors.Is(err, apiclient.ErrScriptNotFound) {
						return nil
					}
					return fmt.Errorf("failed to fetch script: %w", err)
				}
				tmp, err := os.CreateTemp("", "knot-run-script-*.py")
				if err != nil {
					return fmt.Errorf("failed to create temp script: %w", err)
				}
				defer os.Remove(tmp.Name())
				if _, err := tmp.WriteString(fetched); err != nil {
					tmp.Close()
					return fmt.Errorf("failed to write temp script: %w", err)
				}
				tmp.Close()
				scriptFile = tmp.Name()
			}
			return runServe(ctx, cmd, scriptFile, serveClient, serveUserId)
		}

		// Interactive REPL — no script file required.
		if cmd.GetBool("interactive") {
			return runInteractiveMode(ctx, cmd)
		}

		// Non-interactive eval requires a script name or file.
		if scriptArg == "" {
			cmd.ShowHelp()
			return nil
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}
		client.SetTimeout(5 * time.Minute)

		var content string
		if localFile {
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
		wireMethodsRegistrar()

		env, cleanup, err := service.NewRunScriptEvalEnv(argv, client, userId, agentlink.NewScriptLogger("script"), os.Stdout, os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to create script environment: %w", err)
		}
		defer cleanup()

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
