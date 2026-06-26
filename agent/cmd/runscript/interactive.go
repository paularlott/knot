package runscript

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/cli/tui"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/agentlink"
	knotmethods "github.com/paularlott/knot/internal/methods"
	knotscriptling "github.com/paularlott/knot/internal/scriptling"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs/agent"
)

// wireMethodsRegistrar connects knot.methods register()/unregister_all() to
// the agent daemon via the agentlink command socket, so scripts executed in the
// CLI process (eval or interactive) publish methods to the daemon — the same
// path as `knot methods register`.
func wireMethodsRegistrar() {
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
	knotscriptling.SetMethodsUnregisterAll(func() error {
		var resp agentlink.RegisterMethodsResponse
		if err := agentlink.SendWithResponseMsg(agentlink.CommandUnregisterMethods, nil, &resp); err != nil {
			return err
		}
		if !resp.Success {
			return errors.New(resp.Error)
		}
		return nil
	})
}

// runInteractiveMode builds the full knot run-script environment (same library
// surface as the eval path) and starts a REPL, mirroring the scriptling CLI's
// --interactive mode. Each submitted line is evaluated in the shared scriptling
// environment with streaming output to the TUI; Esc interrupts the running
// evaluation.
func runInteractiveMode(ctx context.Context, cmd *cli.Command) error {
	client, err := cmdutil.GetClient(cmd)
	if err != nil {
		return fmt.Errorf("failed to create API client: %w", err)
	}
	client.SetTimeout(5 * time.Minute)

	var userId string
	if user, err := client.WhoAmI(ctx); err == nil {
		userId = user.Id
	}

	wireMethodsRegistrar()

	env, cleanup, err := service.NewRunScriptEvalEnv([]string{"knot"}, client, userId, agentlink.NewScriptLogger("script"), os.Stdout, os.Stdin)
	if err != nil {
		return fmt.Errorf("failed to create script environment: %w", err)
	}
	defer cleanup()

	// Register the interactive agent library so scriptling.ai.agent.interact is
	// available in the REPL, matching the scriptling CLI's interactive mode.
	_ = agent.RegisterInteract(env)

	return runInteractive(env)
}

// runInteractive drives the TUI REPL over an existing scriptling environment.
func runInteractive(env *scriptling.Scriptling) error {
	var (
		t         *tui.TUI
		cancel    context.CancelFunc
		runningMu sync.Mutex
	)

	t = tui.New(tui.Config{
		HideHeaders: true,
		StatusRight: "Ctrl+C to exit · Esc to interrupt",
		Commands: []*tui.Command{
			{
				Name:        "exit",
				Description: "Exit interactive mode",
				Handler:     func(_ string) { t.Exit() },
			},
			{
				Name:        "clear",
				Description: "Clear output",
				Handler:     func(_ string) { t.ClearOutput() },
			},
		},
		OnEscape: func() {
			runningMu.Lock()
			if cancel != nil {
				cancel()
			}
			runningMu.Unlock()
		},
		OnSubmit: func(line string) {
			t.AddMessage(tui.RoleUser, line)

			evalCtx, c := context.WithCancel(context.Background())
			runningMu.Lock()
			cancel = c
			runningMu.Unlock()

			t.StartStreaming()
			t.StartSpinner("Esc to interrupt")
			env.SetOutputWriter(&streamWriter{t: t})

			go func() {
				defer func() {
					env.SetOutputWriter(nil)
					runningMu.Lock()
					cancel = nil
					runningMu.Unlock()
					c()
					t.StopSpinner()
					t.StreamComplete()
				}()
				result, err := env.EvalWithContext(evalCtx, line)
				if err != nil {
					if evalCtx.Err() == nil {
						t.StreamChunk(err.Error())
					}
					return
				}
				if result != nil && result.Inspect() != "None" && !t.IsStreaming() {
					t.AddMessage(tui.RoleAssistant, result.Inspect())
				}
			}()
		},
	})

	t.AddMessage(tui.RoleSystem, tui.Styled(t.Theme().Text, "knot run-script")+"\n"+tui.Styled(t.Theme().Primary, "v"+build.Version))
	return t.Run(context.Background())
}

// streamWriter forwards script output to the TUI as streaming chunks.
type streamWriter struct{ t *tui.TUI }

func (w *streamWriter) Write(p []byte) (int, error) {
	w.t.StreamChunk(string(p))
	return len(p), nil
}
