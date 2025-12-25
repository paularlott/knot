package port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var StopPortForwardCmd = &cli.Command{
	Name:        "stop",
	Usage:       "Stop a port forward",
	Description: "Stop an active port forward by local port number.",
	Arguments: []cli.Argument{
		&cli.IntArg{
			Name:     "local-port",
			Usage:    "The local port to stop forwarding",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		localPort := cmd.GetIntArg("local-port")
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("invalid local port number, must be between 1 and 65535")
		}

		request := agentlink.StopPortForwardRequest{
			LocalPort: uint16(localPort),
		}

		err := agentlink.SendWithResponseMsg(agentlink.CommandStopPortForward, &request, nil)
		if err != nil {
			return fmt.Errorf("error stopping port forward: %w", err)
		}

		fmt.Printf("Port forward on port %d stopped.\n", localPort)
		return nil
	},
}
