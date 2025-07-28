package commands_forward

import (
	"context"
	"fmt"
	"strconv"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/proxy"
	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var PortCmd = &cli.Command{
	Name:        "port",
	Usage:       "Forwards a port into a space",
	Description: `Forwards a local port to a remote container running the agent via the proxy server.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "listen",
			Usage:    "The local port to listen on",
			Required: true,
		},
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to connect to",
			Required: true,
		},
		&cli.StringArg{
			Name:     "port",
			Usage:    "The remote port to connect to",
			Required: false,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		port, err := strconv.Atoi(cmd.GetStringArg("port"))
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535")
		}

		proxy.RunTCPForwarderViaAgent(cfg.WsServer, util.FixListenAddress(cmd.GetStringArg("listen")), cmd.GetStringArg("space"), port, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		return nil
	},
}
