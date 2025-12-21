package port

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentlink"

	"github.com/paularlott/cli"
)

var ForwardPortCmd = &cli.Command{
	Name:        "forward",
	Usage:       "Forward a local port to a port in another space",
	Description: "Forward a local port to a port in another space within the same zone.",
	Arguments: []cli.Argument{
		&cli.IntArg{
			Name:     "local-port",
			Usage:    "The local port to listen on",
			Required: true,
		},
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the target space",
			Required: true,
		},
		&cli.IntArg{
			Name:     "remote-port",
			Usage:    "The remote port to connect to",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		localPort := cmd.GetIntArg("local-port")
		if localPort < 1 || localPort > 65535 {
			return fmt.Errorf("invalid local port number, must be between 1 and 65535")
		}

		remotePort := cmd.GetIntArg("remote-port")
		if remotePort < 1 || remotePort > 65535 {
			return fmt.Errorf("invalid remote port number, must be between 1 and 65535")
		}

		request := agentlink.ForwardPortRequest{
			LocalPort:  uint16(localPort),
			Space:      cmd.GetStringArg("space"),
			RemotePort: uint16(remotePort),
		}

		var response agentlink.RunCommandResponse
		err := agentlink.SendWithResponseMsg(agentlink.CommandForwardPort, &request, &response)
		if err != nil {
			return fmt.Errorf("error requesting port forward: %w", err)
		}

		if !response.Success {
			return fmt.Errorf("%s", response.Error)
		}

		fmt.Printf("Port forward established: %d -> %s:%d\n", localPort, cmd.GetStringArg("space"), remotePort)
		return nil
	},
}
