package runscript

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/scriptling"
	"github.com/paularlott/scriptling/extlibs"
	"github.com/paularlott/scriptling/scriptling-cli/bootstrap"
	"github.com/paularlott/scriptling/scriptling-cli/server"
)

// serveFlags are the run-script flags that put it into a long-running server
// mode, mirroring the scriptling CLI's serve modes. Container runtime support
// is deliberately excluded (no docker/podman sockets are ever wired up).
var serveFlags = []cli.Flag{
	&cli.BoolFlag{
		Name:  "json-rpc",
		Usage: "Run the script as a JSON-RPC server over stdin/stdout.",
	},
	&cli.StringFlag{
		Name:  "listen",
		Usage: "Run the script as an HTTP server on the given address (e.g. :8080).",
	},
	&cli.StringFlag{
		Name:  "mcp-tools",
		Usage: "Run as an MCP server exposing tools from the given directory (implies HTTP).",
	},
	&cli.BoolFlag{
		Name:  "mcp-exec",
		Usage: "Enable the MCP code-execution tool (used with an HTTP/MCP server).",
	},
	&cli.StringFlag{
		Name:  "web-root",
		Usage: "Directory (or .zip) of static files to serve alongside an HTTP server.",
	},
	&cli.StringFlag{
		Name:  "bearer-token",
		Usage: "Require this bearer token on incoming HTTP/MCP requests.",
	},
	&cli.StringFlag{
		Name:  "kv-storage",
		Usage: "Path for the persistent key-value store (in-memory if unset).",
	},
	&cli.StringSliceFlag{
		Name:  "allowed-path",
		Usage: "Restrict filesystem access to these paths (may be repeated).",
	},
	&cli.StringSliceFlag{
		Name:  "disable-lib",
		Usage: "Disable a built-in library by name (may be repeated).",
	},
	&cli.StringSliceFlag{
		Name:  "lib-dir",
		Usage: "Additional directory to load libraries from (may be repeated).",
	},
	&cli.StringFlag{
		Name:  "tls-cert",
		Usage: "PEM certificate file for HTTPS.",
	},
	&cli.StringFlag{
		Name:  "tls-key",
		Usage: "PEM key file for HTTPS.",
	},
	&cli.BoolFlag{
		Name:  "tls-generate",
		Usage: "Generate a self-signed certificate for HTTPS.",
	},
}

// serveRequested reports whether any server-mode flag was supplied.
func serveRequested(cmd *cli.Command) bool {
	return cmd.GetBool("json-rpc") ||
		cmd.GetString("listen") != "" ||
		cmd.GetString("mcp-tools") != "" ||
		cmd.GetBool("mcp-exec")
}

// runServe runs the script in a long-running server mode using the scriptling
// server runtime. scriptFile must be a path on disk (the caller materialises
// named scripts to a temp file). Container libraries are always disabled and no
// docker/podman sockets are configured, so served scripts cannot drive a
// container runtime.
func runServe(ctx context.Context, cmd *cli.Command, scriptFile string, client *apiclient.ApiClient, userId string) error {
	// Route the server's own logs and the Python logging library (registered
	// via setup.Scriptling using this Log) to the agent uplink when running
	// inside a space, otherwise to stderr.
	server.Log = agentlink.NewScriptLogger("script")

	// Resolve co-located handler modules relative to the script (handlers are
	// referenced by "module.func" strings), matching the scriptling CLI.
	baseDir, err := bootstrap.BaseDir(scriptFile)
	if err != nil {
		return fmt.Errorf("failed to resolve script directory: %w", err)
	}

	cfg := server.ServerConfig{
		ScriptFile:    scriptFile,
		Address:       cmd.GetString("listen"),
		LibDirs:       bootstrap.BuildLibDirs(baseDir, cmd.GetStringSlice("lib-dir")),
		AllowedPaths:  cmd.GetStringSlice("allowed-path"),
		DisabledLibs:  append(cmd.GetStringSlice("disable-lib"), extlibs.ContainerLibraryName),
		BearerToken:   cmd.GetString("bearer-token"),
		WebRoot:       cmd.GetString("web-root"),
		KVStoragePath: cmd.GetString("kv-storage"),
		MCPToolsDir:   cmd.GetString("mcp-tools"),
		MCPExecTool:   cmd.GetBool("mcp-exec"),
		TLSCert:       cmd.GetString("tls-cert"),
		TLSKey:        cmd.GetString("tls-key"),
		TLSGenerate:   cmd.GetBool("tls-generate"),
		// DockerSock/PodmanSock intentionally left empty — no container runtime.
	}

	// Expose knot's libraries (knot.apiclient, knot.ai, knot.* Python libs…) to
	// every served handler via the scriptling server's ExtraLibs hook. Requires
	// a server connection; without one (e.g. serving a local file standalone)
	// the script still runs, just without the knot libraries.
	if client != nil {
		cfg.ExtraLibs = func(p *scriptling.Scriptling) {
			service.RegisterKnotServeLibraries(p, client, userId)
		}
	}

	if cmd.GetBool("json-rpc") {
		return server.RunJSONRPCServer(ctx, cfg)
	}

	if cfg.Address == "" {
		cfg.Address = ":8080"
	}
	return server.RunServer(ctx, cfg)
}
