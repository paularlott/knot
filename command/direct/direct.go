package commands_direct

import (
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var DirectCmd = &cli.Command{
	Name:        "direct",
	Usage:       "Direct connection to a service",
	Description: "Create a direct connection from a local port to a remote service looking up the IP and port via SRV records.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:       "nameserver",
			Usage:      "The address of the nameserver to use for SRV lookups, can be given multiple times.",
			ConfigPath: []string{"resolver", "nameservers"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_NAMESERVERS"},
			Global:     true,
		},
	},
	Commands: []*cli.Command{
		SshCmd,
		PortCmd,
		LookupCmd,
	},
}
