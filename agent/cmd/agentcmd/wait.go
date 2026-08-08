package agentcmd

import (
	"context"
	"fmt"
	"time"

	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/log"
)

// agentWaitCmd blocks until the agent daemon is running and accepting commands
// on its unix socket (~/.knot/agent.sock), then exits. It is used by the
// container entrypoint — right after `knot agent start` — to gate user startup
// scripts on agent readiness. Because the resident DNS resolver binds :53
// before the command socket comes up, socket readiness also guarantees the
// resolver is up when agent DNS is enabled.
var agentWaitCmd = &cli.Command{
	Name:        "wait-for-start",
	Usage:       "Block until the agent daemon is running and accepting commands, then exit.",
	Description: `Used by the container entrypoint (after 'knot agent start') to wait for the agent to be ready before launching user processes. Exits non-zero if the agent does not become ready within the timeout.`,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:         "timeout",
			Usage:        "Maximum number of seconds to wait for the agent.",
			ConfigPath:   []string{"agent.wait_timeout"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_WAIT_TIMEOUT"},
			DefaultValue: 60,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		timeout := time.Duration(cmd.GetInt("timeout")) * time.Second
		deadline := time.Now().Add(timeout)
		logger := log.WithGroup("agent")
		for {
			if agentlink.IsAgentRunning() {
				return nil
			}
			if time.Now().After(deadline) {
				logger.Error("timed out waiting for agent to start", "timeout", timeout)
				return fmt.Errorf("timed out waiting for agent to start after %s", timeout)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	},
}
