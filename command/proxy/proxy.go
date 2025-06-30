package command_proxy

import (
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/cli"
)

var ProxyCmd = &cli.Command{
	Name:        "proxy",
	Usage:       "Proxy a connection",
	Description: "Proxy a connection from the local host to a remote destination via the proxy server.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:       "server",
			Aliases:    []string{"s"},
			Usage:      "The address of the remote server to proxy through.",
			ConfigPath: []string{"client.server"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			Global:     true,
		},
		&cli.StringFlag{
			Name:       "token",
			Aliases:    []string{"t"},
			Usage:      "The token to use for authentication.",
			ConfigPath: []string{"client.token"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
			Global:     true,
		},
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
		SshCmd,
		PortCmd,
		LookupCmd,
	},
}
