package command_tunnel

import (
	"github.com/paularlott/cli"
)

// TunnelCmd is the agent binary's tunnel command. It offers foreground and
// daemon web tunnels plus stop/list for agent-owned tunnels.
var TunnelCmd = &cli.Command{
	Name:        "tunnel",
	Usage:       "Open a tunnel",
	Description: `Open a tunnel to expose a local port on the internet, or manage daemon-owned tunnels.

When run without --daemon the tunnel lives for the life of this command (until
Ctrl-C). With --daemon the tunnel is handed to the knot agent and kept alive for
the life of the agent; the command returns immediately.

The tunnel is exposed as <user>--<tunnel_name>.<domain>.`,
	MaxArgs: cli.NoArgs,
	Commands: []*cli.Command{
		HttpTunnelCmd,
		HttpsTunnelCmd,
		StopTunnelCmd,
		ListTunnelCmd,
	},
}

// DesktopTunnelCmd is the desktop (main knot binary) tunnel command. It only
// offers foreground web tunnels — there is no knot agent running on a
// workstation to own a daemon tunnel, so --daemon, stop and list are excluded.
var DesktopTunnelCmd = &cli.Command{
	Name:        "tunnel",
	Usage:       "Open a tunnel",
	Description: `Open a tunnel to expose a local port on the internet.

The tunnel lives for the life of this command (until Ctrl-C). It is exposed as
<user>--<tunnel_name>.<domain>.`,
	MaxArgs: cli.NoArgs,
	Commands: []*cli.Command{
		DesktopHttpTunnelCmd,
		DesktopHttpsTunnelCmd,
	},
}
