package command_spaces

import (
	"github.com/paularlott/cli"
)

// PortCmd is the `knot space port` group: remote control of a space's
// inter-space port forwards (forward/list/stop), driven via the server relay
// (/space-io/{space}/port/*).
var PortCmd = &cli.Command{
	Name:        "port",
	Usage:       "Manage a space's port forwards",
	Description: `Manage port forwards from a space to other spaces.`,
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		PortForwardCmd,
		PortListCmd,
		PortStopCmd,
	},
}
