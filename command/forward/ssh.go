package commands_forward

import (
	"context"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/proxy"

	"github.com/paularlott/cli"
)

var SshCmd = &cli.Command{
	Name:        "ssh",
	Usage:       "Forward SSH to a space",
	Description: `Forwards a SSH connection to a container running the agent via the proxy server.`,
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Usage:    "The name of the space to connect to",
			Required: true,
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		space := cmd.GetStringArg("space")
		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)

		return proxy.RunSSHForwarderViaAgent(cfg.WsServer, space, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
	},
}
