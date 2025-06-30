package command_proxy

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
	Name:  "port",
	Usage: "Forward a local port to a remote service",
	Description: `Forwards a local port to a remote server and port via the proxy server.

If <port> is not given then the remote port is found via a DNS SRV lookup against the service name.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "listen",
			Usage:    "The local port to listen on",
			Required: true,
		},
		&cli.StringArg{
			Name:     "service",
			Usage:    "The name of the service to connect to",
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
		var port int
		var err error

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		if cmd.HasArg("port") {
			port, err = strconv.Atoi(cmd.GetStringArg("port"))
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("Invalid port number, port numbers must be between 1 and 65535", 1)
			}
		} else {
			port = 0
		}

		proxy.RunTCPForwarderViaProxy(cfg.WsServer, cfg.ApiToken, util.FixListenAddress(cmd.GetStringArg("listen")), cmd.GetStringArg("service"), port)
		return nil
	},
}
