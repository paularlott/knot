package agent_cmd

import (
	"context"
	"fmt"

	connectcmd "github.com/paularlott/knot/agent/cmd/connect"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var ConnectCmd = &cli.Command{
	Name:        "connect",
	Usage:       "Generate API key",
	Description: "Asks the running agent to generate a new API key on the server and stores it in the local config.",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
			Global:       true,
		},
	},
	Commands: []*cli.Command{
		connectcmd.ConnectListCmd,
		connectcmd.ConnectDeleteCmd,
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		var response agentlink.ConnectResponse

		err := agentlink.SendWithResponseMsg(agentlink.CommandConnect, nil, &response)
		if err != nil {
			return fmt.Errorf("unable to connect to the agent, please check that the agent is running: %w", err)
		}

		if !response.Success {
			return fmt.Errorf("failed to create an API token: %w", err)
		}

		if err := config.SaveConnection("default", response.Server, response.Token, cmd); err != nil {
			return fmt.Errorf("failed to save connection details: %w", err)
		}

		fmt.Println("Successfully connected to server:", response.Server)
		return nil
	},
}
