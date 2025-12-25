package port

import (
	"github.com/paularlott/cli"
)

var PortCmd = &cli.Command{
	Name:        "port",
	Usage:       "Manage port forwards",
	Description: "Manage port forwards between spaces.",
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		ForwardPortCmd,
		ListPortForwardsCmd,
		StopPortForwardCmd,
	},
}
