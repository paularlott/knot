package command_ssh_config

import (
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var SshConfigCmd = &cli.Command{
	Name:        "ssh-config",
	Usage:       "Operate on the .ssh/config file",
	Description: "Operations to perform management of the .ssh/config file.",
	MaxArgs:     cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls_skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
			Global:       true,
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The server alias to use.",
			DefaultValue: "default",
			Global:       true,
		},
	},
	Commands: []*cli.Command{
		SshConfigUpdateCmd,
		SshConfigRemoveCmd,
	},
}
