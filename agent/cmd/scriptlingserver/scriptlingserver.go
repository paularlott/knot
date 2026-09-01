// Package scriptlingserver implements the plugin the scriptling CLI loads
// to use knot libraries and scripts. When scriptling spawns the knot
// executable as a plugin peer it sets SCRIPTLING_PLUGIN_PEER in the
// environment, so a bare `knot` invocation (optionally carrying plugin
// options like --alias from scriptling's --plugin-arg) can divert here
// without a subcommand — the CLI never needs to know one exists.
package scriptlingserver

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/scriptling/pluginfetch"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/scriptling/conversion"
	"github.com/paularlott/scriptling/object"
	"github.com/paularlott/scriptling/plugin"
)

// PeerEnvVar is the environment variable scriptling sets on spawned plugin
// peers. A knot process that sees it diverts to the plugin server instead
// of running the CLI, desktop or agent.
const PeerEnvVar = "SCRIPTLING_PLUGIN_PEER"

// ShouldAutoStart reports whether this process was spawned by scriptling as
// a plugin peer: scriptling sets SCRIPTLING_PLUGIN_PEER (carrying its
// version) on the knot executable it spawns.
func ShouldAutoStart() bool {
	return os.Getenv(PeerEnvVar) != ""
}

// AutoStartCmd returns the command that serves the plugin when scriptling
// spawns the knot binary. Plugin arguments passed via scriptling's
// --plugin-arg arrive as this process's argv, so the standard CLI machinery
// parses them: --alias selects the configured server alias to talk to
// (default "default"), and --server/--token address a server outright.
func AutoStartCmd() *cli.Command {
	return newPluginCmd(servePeer)
}

// newPluginCmd builds the plugin command around a run function so tests can
// capture the parsed command without starting the plugin server.
func newPluginCmd(run func(context.Context, *cli.Command) error) *cli.Command {
	var configFile = config.CONFIG_FILE
	return &cli.Command{
		Name:        "knot",
		Usage:       "knot plugin server for the scriptling CLI",
		Description: "Serves knot libraries, scripts and API access to the scriptling CLI. Auto-started when scriptling spawns the knot binary as a plugin peer; not for direct use.",
		ConfigFile: cli_toml.NewConfigFile(&configFile, func() []string {
			return []string{
				".",
				os.Getenv("HOME"),
				filepath.Join(os.Getenv("HOME"), ".config", "knot"),
			}
		}),
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:         "alias",
				Aliases:      []string{"a"},
				Usage:        "The configured server alias to talk to.",
				EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_ALIAS"},
				DefaultValue: "default",
			},
			&cli.StringFlag{
				Name:    "server",
				Usage:   "The address of the knot server.",
				EnvVars: []string{config.CONFIG_ENV_PREFIX + "_SERVER"},
			},
			&cli.StringFlag{
				Name:    "token",
				Usage:   "API token for the knot server.",
				EnvVars: []string{config.CONFIG_ENV_PREFIX + "_TOKEN"},
			},
		},
		MaxArgs:        cli.NoArgs,
		DisableHelp:    true,
		DisableVersion: true,
		Run:            run,
	}
}

// servePeer resolves the connection and serves the plugin. In a space the
// agentlink socket has the connection and the flags don't apply; on the
// desktop --server/--token win when given, else the --alias section of the
// config file.
func servePeer(ctx context.Context, cmd *cli.Command) error {
	peerVersion := os.Getenv(PeerEnvVar)
	os.Unsetenv(PeerEnvVar)

	if peerVersion == "" {
		return fmt.Errorf("knot plugin: %s not set; this binary is being run outside the scriptling plugin protocol", PeerEnvVar)
	}
	if err := checkPeerVersion(peerVersion); err != nil {
		return fmt.Errorf("knot plugin: incompatible scriptling %s: %w", peerVersion, err)
	}

	var client *apiclient.ApiClient
	var err error

	if agentlink.IsAgentRunning() {
		// In a space: the agentlink socket has the connection; --alias does
		// not apply.
		server, token, linkErr := agentlink.GetConnectionInfo()
		if linkErr != nil {
			return fmt.Errorf("knot plugin: agent connection: %w", linkErr)
		}
		client, err = apiclient.NewClient(server, token, true)
	} else {
		// Desktop: an explicit --server/--token (flag or env) wins, else the
		// alias section of the config file.
		server, token := cmd.GetString("server"), cmd.GetString("token")
		if server != "" && token != "" {
			client, err = apiclient.NewClient(server, token, true)
		} else {
			alias := cmd.GetString("alias")
			cfg := config.GetServerAddr(alias, cmd)
			if cfg == nil || cfg.HttpServer == "" {
				return fmt.Errorf("knot plugin: no server configured for alias %q (run 'knot connect' first, or set KNOT_SERVER and KNOT_TOKEN)", alias)
			}
			client, err = apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, true)
		}
	}
	if err != nil {
		return fmt.Errorf("knot plugin: creating client: %w", err)
	}

	return serve(client)
}

// MinPeerVersion is the earliest scriptling version whose plugin protocol
// this build of knot can serve. The fetcher contract (one scheme, no
// manifest, bytes in/out) landed in 0.23.0.
const MinPeerVersion = "0.23.0"

// checkPeerVersion reports whether the scriptling version that spawned us
// is one we can serve.
func checkPeerVersion(peerVersion string) error {
	min, err := semver(MinPeerVersion)
	if err != nil {
		return fmt.Errorf("bad minimum version %q: %v", MinPeerVersion, err)
	}
	got, err := semver(peerVersion)
	if err != nil {
		return fmt.Errorf("cannot parse version %q: %v", peerVersion, err)
	}
	if got[0] < min[0] || (got[0] == min[0] && got[1] < min[1]) {
		return fmt.Errorf("need at least %s", MinPeerVersion)
	}
	return nil
}

// semver extracts (major, minor) from a version string like "0.23.0".
func semver(v string) ([2]int, error) {
	var major, minor int
	if _, err := fmt.Sscanf(v, "%d.%d", &major, &minor); err != nil {
		return [2]int{}, err
	}
	return [2]int{major, minor}, nil
}

// serve builds and runs the plugin server on stdin/stdout.
func serve(client *apiclient.ApiClient) error {
	server := plugin.NewServer("knot", build.Version, "Knot libraries, scripts and API for scriptling")

	fetcher := pluginfetch.NewFetcher(client)
	server.RegisterFetcher("knot", fetcherAdapter{fetcher})

	registerAPIFunctions(server, client.GetRESTClient())

	return server.Run()
}

// fetcherAdapter bridges the local pluginfetch.Fetcher (which avoids
// importing the scriptling module) to the plugin.Fetcher interface.
type fetcherAdapter struct{ *pluginfetch.Fetcher }

func (a fetcherAdapter) Read(ctx context.Context, source, path string) ([]byte, error) {
	return a.Fetcher.Read(ctx, source, path)
}

func (a fetcherAdapter) Glob(ctx context.Context, source, pattern string) ([]plugin.FetchEntry, error) {
	tree, err := a.Fetcher.Tree(ctx)
	if err != nil {
		return nil, err
	}
	out := []plugin.FetchEntry{}
	for _, e := range tree {
		if plugin.MatchGlob(pattern, e.Name) {
			out = append(out, plugin.FetchEntry{Name: e.Name, IsDir: e.IsDir})
		}
	}
	return out, nil
}

// registerAPIFunctions exposes the REST API through plugin functions, so
// the variant knot.apiclient routes calls through this process and the
// token never reaches scripts.
func registerAPIFunctions(server *plugin.Server, client rest.RESTClient) {
	doRequest := func(ctx context.Context, method, path string, kwargs object.Kwargs, hasBody bool) object.Object {
		var result interface{}
		var statusCode int
		var apiErr error

		switch method {
		case "GET":
			if params := extractQuery(kwargs); params != "" {
				if strings.Contains(path, "?") {
					path += "&" + params
				} else {
					path += "?" + params
				}
			}
			statusCode, apiErr = client.GetJSON(ctx, path, &result)
		case "POST":
			var body interface{}
			if hasBody {
				if b := kwargs.Get("body"); b != nil {
					if _, isNull := b.(*object.Null); !isNull {
						body = conversion.ToGo(b)
					}
				}
			}
			statusCode, apiErr = client.PostJSON(ctx, path, body, &result, 0)
		case "PUT":
			var body interface{}
			if hasBody {
				if b := kwargs.Get("body"); b != nil {
					if _, isNull := b.(*object.Null); !isNull {
						body = conversion.ToGo(b)
					}
				}
			}
			statusCode, apiErr = client.PutJSON(ctx, path, body, &result, 0)
		case "DELETE":
			statusCode, apiErr = client.Delete(ctx, path, nil, &result, 0)
		}

		// HTTP 200 with an empty body yields io.EOF from the JSON decoder —
		// the knot API does this for start/stop and other action endpoints.
		// The embedded apiclient tolerates it (apiclient_library.go:91);
		// anything else is a real error.
		if apiErr != nil && apiErr != io.EOF {
			return &object.Error{Message: fmt.Sprintf("API error (HTTP %d): %v", statusCode, apiErr)}
		}
		if statusCode >= 400 {
			return &object.Error{Message: fmt.Sprintf("API error (HTTP %d)", statusCode)}
		}
		if result == nil {
			return &object.Null{}
		}
		return conversion.FromGo(result)
	}

	register := func(name, method string, hasBody bool) {
		server.RegisterBuiltin(name, func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
			if len(args) < 1 {
				return &object.Error{Message: fmt.Sprintf("%s: missing path argument", name)}
			}
			path, err := args[0].AsString()
			if err != nil {
				return &object.Error{Message: fmt.Sprintf("%s: path: %v", name, err)}
			}
			return doRequest(ctx, method, path, kwargs, hasBody)
		})
	}

	register("api_get", "GET", false)
	register("api_post", "POST", true)
	register("api_put", "PUT", true)
	register("api_delete", "DELETE", false)

	server.RegisterBuiltin("connection_info", func(ctx context.Context, kwargs object.Kwargs, args ...object.Object) object.Object {
		baseURL := client.GetBaseURL()
		token := client.GetAuthToken()
		aiURL := strings.TrimSuffix(baseURL, "/") + "/v1"
		return object.NewStringDict(map[string]object.Object{
			"url":         object.NewString(baseURL),
			"token":       object.NewString(token),
			"ai_url":      object.NewString(aiURL),
			"ai_token":    object.NewString(token),
			"ai_model":    object.NewString(""),
			"ai_provider": object.NewString("openai"),
		})
	})
}

func extractQuery(kwargs object.Kwargs) string {
	params := kwargs.Get("params")
	if params == nil {
		return ""
	}
	if _, isNull := params.(*object.Null); isNull {
		return ""
	}
	dict, ok := params.(*object.Dict)
	if !ok {
		return ""
	}
	var parts []string
	for _, pair := range dict.Pairs {
		parts = append(parts, pair.Key.Inspect()+"="+pair.Value.Inspect())
	}
	return strings.Join(parts, "&")
}
