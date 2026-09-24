package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"

	"github.com/paularlott/cli"
	mcp "github.com/paularlott/mcp"
	"github.com/paularlott/mcp/pool"
)

var McpCmd = &cli.Command{
	Name:    "mcp",
	Usage:   "Serve the knot MCP server over stdio, proxying a remote knot server",
	Description: `Runs knot as a local MCP server on stdio, forwarding tool calls to a remote knot server's /mcp endpoint over HTTP.

Intended for MCP hosts that launch the server as a subprocess and only speak stdio, e.g. Claude Desktop:

    {"mcpServers": {"knot": {"command": "knot", "args": ["mcp", "--alias", "default"]}}}

This sidesteps the host's HTTPS-only rule for remote servers: the subprocess talks plain HTTP to the knot server, and authentication comes from the connection config rather than the host's — the API token never appears in the host's configuration.

The connection is resolved like other client commands: an explicit --server/--token pair, else inside a space the agent socket, else the --alias section written by 'knot connect'.

Stdout is the MCP protocol; all diagnostics go to stderr.`,
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "server",
			Aliases: []string{"s"},
			Usage:   "The address of the remote knot server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
		},
		&cli.StringFlag{
			Name:    "token",
			Aliases: []string{"t"},
			Usage:   "API token for the remote knot server.",
			EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
		},
		&cli.StringFlag{
			Name:         "alias",
			Aliases:      []string{"a"},
			Usage:        "The configured server alias to talk to.",
			DefaultValue: "default",
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to the server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:  "show-all",
			Usage: "Also expose discoverable tools, not just native ones.",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		// Errors must reach the host's logs without polluting stdout — the
		// protocol stream — so print to stderr and exit here rather than
		// returning through main's stdout error print.
		if err := serveMCP(ctx, cmd); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return nil
	},
}

// serveMCP resolves the connection and serves the stdio proxy until stdin
// closes or the process is interrupted.
func serveMCP(ctx context.Context, cmd *cli.Command) error {
	cfg, err := resolveMCPServerAddr(cmd)
	if err != nil {
		return err
	}

	server, client, err := buildMCPServer(cfg, cmd.GetBool("tls-skip-verify"), cmd.GetBool("show-all"))
	if err != nil {
		return err
	}
	defer client.Close()

	log.WithGroup("mcp").Info("serving MCP stdio proxy", "server", cfg.HttpServer)

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	return server.ServeStdio(ctx)
}

// resolveMCPServerAddr finds the knot server to proxy: an explicit
// --server/--token pair wins, else inside a space the agentlink socket carries
// the connection, else the --alias section of the config file. Same precedence
// as the scriptling plugin's desktop resolution.
func resolveMCPServerAddr(cmd *cli.Command) (*config.ServerAddr, error) {
	if server, token := cmd.GetString("server"), cmd.GetString("token"); server != "" && token != "" {
		return config.NewServerAddr(server, token), nil
	}

	if agentlink.IsAgentRunning() {
		server, token, err := agentlink.GetConnectionInfo()
		if err != nil {
			return nil, fmt.Errorf("agent connection: %w", err)
		}
		return config.NewServerAddr(server, token), nil
	}

	alias := cmd.GetString("alias")
	if cfg, ok := config.LookupServerAddr(alias, cmd); ok {
		return cfg, nil
	}
	return nil, fmt.Errorf("no server configured for alias %q (run 'knot connect' first, or set KNOT_SERVER and KNOT_TOKEN)", alias)
}

// buildMCPServer composes the proxy: a local MCP server whose single remote is
// the knot server's /mcp endpoint. An empty namespace keeps the remote's tool
// names unchanged (no knot__ prefix), --show-all rides along as a request
// header so the remote includes discoverable tools, and notifications are on so
// upstream listChanged events reach the host.
func buildMCPServer(cfg *config.ServerAddr, tlsSkipVerify, showAll bool) (*mcp.Server, *mcp.Client, error) {
	url := strings.TrimSuffix(cfg.HttpServer, "/") + "/mcp"

	var headers map[string]string
	if showAll {
		headers = map[string]string{mcp.ShowAllHeader: "true"}
	}

	var httpPool pool.HTTPPool
	if tlsSkipVerify {
		httpPool = pool.NewPool(&pool.PoolConfig{InsecureSkipVerify: true})
	}

	client := mcp.NewClientWithPool(url, mcp.NewBearerTokenAuth(cfg.ApiToken), "", httpPool, mcp.WithClientRequestHeaders(headers))
	client.EnableNotifications()
	// Advertise this client's own support for the MCP Apps extension to the
	// remote knot server. Without this, a spec-conformant remote server that
	// only attaches _meta.ui for clients that declared
	// capabilities.extensions[io.modelcontextprotocol/ui] has no way to know
	// this proxy can forward one, and silently serves a plain-text-only tool
	// instead — MCP Apps then quietly never works, with no error to explain why.
	client.DeclareExtension(mcp.UIAppsExtensionID, map[string]any{
		"mimeTypes": []string{mcp.UIAppMimeType},
	})

	server := mcp.NewServer("knot", build.Version)
	// Mirror the same declaration on the server side of this stdio proxy, so
	// the connecting host (e.g. Claude Desktop) sees accurate capabilities —
	// this proxy forwards whatever _meta.ui the upstream knot server sends,
	// unconditionally, regardless of what the host itself declares.
	server.DeclareExtension(mcp.UIAppsExtensionID, map[string]any{
		"mimeTypes": []string{mcp.UIAppMimeType},
	})
	if err := server.RegisterRemoteServer(client); err != nil {
		client.Close()
		return nil, nil, fmt.Errorf("registering knot server %s: %w", cfg.HttpServer, err)
	}

	return server, client, nil
}
