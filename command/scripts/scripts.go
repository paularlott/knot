package scripts

import (
	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/config"
)

var ScriptsCmd = &cli.Command{
	Name:        "script",
	Usage:       "Manage scripts",
	Description: "Manage scripts from the command line.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			Global:  true,
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "The token to use for authentication.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
			Global:  true,
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification.",
			ConfigPath:   []string{"tls.skip_verify"},
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
		listCmd,
		showCmd,
		deleteCmd,
		runCmd,
	},
}
