package cmdutil

import (
	"fmt"
	"os"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
)

// ExplicitServerAddr returns the server the user explicitly targeted via a
// --server and --token pair (flag or env), or via an --alias that resolves in
// the config file. It returns nil when neither is given, so inside a space
// callers can fall back to the server that owns the space.
//
// Note the both-non-empty rule for the flags: spaces always have KNOT_SERVER
// set to the owning server's URL, so a token alone must never count as an
// explicit target. An --alias is only honoured when actually given on the
// command line — the "default" alias is never consulted implicitly, so a
// config file carried into a space cannot silently redirect tunnels.
func ExplicitServerAddr(cmd *cli.Command) *config.ServerAddr {
	if server, token := cmd.GetString("server"), cmd.GetString("token"); server != "" && token != "" {
		return config.NewServerAddr(server, token)
	}
	if cmd.HasFlag("alias") {
		if cfg, ok := config.LookupServerAddr(cmd.GetString("alias"), cmd); ok {
			return cfg
		}
	}
	return nil
}

func GetServerAddr(cmd *cli.Command) *config.ServerAddr {
	if agentlink.IsAgentRunning() {
		// In a space: an explicit --server/--token or --alias tunnels via
		// that server from this process; otherwise the agentlink socket
		// carries the connection to the server that owns the space.
		if cfg := ExplicitServerAddr(cmd); cfg != nil {
			return cfg
		}

		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: couldn't get the server connection from the space agent: %v\n", err)
			return nil
		}

		return config.NewServerAddr(server, token)
	}

	alias := cmd.GetString("alias")
	return config.GetServerAddr(alias, cmd)
}

func GetClient(cmd *cli.Command) (*apiclient.ApiClient, error) {
	if agentlink.IsAgentRunning() {
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			return nil, fmt.Errorf("couldn't get the server connection from the space agent: %w", err)
		}

		client, err := apiclient.NewClient(server, token, true)
		if err != nil {
			return nil, fmt.Errorf("couldn't create the API client for %s: %w", server, err)
		}

		return client, nil
	}

	alias := cmd.GetString("alias")
	cfg := config.GetServerAddr(alias, cmd)

	if cfg.HttpServer == "" {
		return nil, ErrNoServer
	}

	client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
	if err != nil {
		return nil, fmt.Errorf("couldn't create the API client for %s: %w", cfg.HttpServer, err)
	}

	return client, nil
}

// CleanAPIError strips the transport framing the REST client wraps around
// non-2xx responses ("unexpected status code: 400: …") and returns just the
// server's error message.
func CleanAPIError(err error) string {
	if he := apiclient.AsHTTPError(err); he != nil {
		return strings.Replace(err.Error(), he.Error(), he.Message(), 1)
	}
	const prefix = "unexpected status code: "
	msg := err.Error()
	if strings.HasPrefix(msg, prefix) {
		rest := msg[len(prefix):]
		if i := strings.Index(rest, ": "); i >= 0 {
			return rest[i+2:]
		}
	}
	return msg
}

// CleanErr wraps err so its text is CleanAPIError's (the server's message
// without the REST client's framing) while errors.As still finds the
// underlying HTTPError, letting FormatError add the status and a hint.
func CleanErr(err error) error {
	if err == nil {
		return nil
	}
	return &cleanedError{err: err}
}

type cleanedError struct{ err error }

func (e *cleanedError) Error() string { return CleanAPIError(e.err) }
func (e *cleanedError) Unwrap() error { return e.err }

// ClientFlags returns the flags that choose the server a command talks to: an
// explicit --server and --token, else the alias (default "default") from the
// config file's client.connection section. Commands that already use -s, -t
// or -a for something else take them without short names.
func ClientFlags(short bool) []cli.Flag {
	var serverAliases, tokenAliases, aliasAliases []string
	if short {
		serverAliases, tokenAliases, aliasAliases = []string{"s"}, []string{"t"}, []string{"a"}
	}
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: serverAliases,
			Usage:   "The address of the remote server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			Global:  true,
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: tokenAliases,
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
			Aliases:      aliasAliases,
			Usage:        "The server alias to use from the config file.",
			DefaultValue: "default",
			Global:       true,
		},
	}
}
