package command_tunnel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

// parseTunnelCmd executes the http tunnel command's flag surface against the
// given args and config file content, returning the parsed command seen by
// its Run function.
func parseTunnelCmd(t *testing.T, configFile, args string) *cli.Command {
	t.Helper()

	path := filepath.Join(t.TempDir(), "knot.toml")
	if configFile != "" {
		if err := os.WriteFile(path, []byte(configFile), 0600); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
	}

	var captured *cli.Command
	root := &cli.Command{
		Name:        "knot",
		ConfigFile:  cli_toml.NewConfigFile(&path, nil),
		Description: "test root",
		Commands: []*cli.Command{
			{
				Name:        "tunnel",
				Usage:       "Open a tunnel",
				Description: "test tunnel",
				Commands: []*cli.Command{
					{
						Name:        "http",
						Usage:       "Open an http tunnel",
						Description: "test http",
						Arguments: []cli.Argument{
							&cli.IntArg{Name: "port", Usage: "port", Required: true},
							&cli.StringArg{Name: "name", Usage: "name", Required: true},
						},
						MaxArgs: cli.NoArgs,
						Flags:   tunnelBaseFlags(),
						Run: func(ctx context.Context, cmd *cli.Command) error {
							captured = cmd
							return nil
						},
					},
				},
			},
		},
	}

	// The http subcommand declares required positional args; supply dummies.
	oldArgs := os.Args
	os.Args = append([]string{"knot", "tunnel", "http", "8080", "test1"}, strings.Fields(args)...)
	defer func() { os.Args = oldArgs }()

	if err := root.Execute(context.Background()); err != nil {
		t.Fatalf("failed to execute command with args %q: %v", args, err)
	}
	if captured == nil {
		t.Fatal("command did not run")
	}
	return captured
}

const requestTestConfig = `
[client.connection.other]
server = "other.example.com"
token = "tok-other"
`

func TestBuildStartTunnelRequest(t *testing.T) {
	cases := []struct {
		name        string
		config      string
		args        string
		wantServer  string
		wantToken   string
		wantSkipTLS bool
	}{
		{
			name:        "no explicit target uses the space's server",
			config:      requestTestConfig,
			args:        "",
			wantServer:  "",
			wantToken:   "",
			wantSkipTLS: true,
		},
		{
			name:        "server and token flags forwarded normalised",
			config:      "",
			args:        "--server other.example.com --token tok-other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:        "explicit alias forwarded from config",
			config:      requestTestConfig,
			args:        "-a other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:        "alias not in config uses the space's server",
			config:      requestTestConfig,
			args:        "-a unknown",
			wantServer:  "",
			wantToken:   "",
			wantSkipTLS: true,
		},
		{
			name:        "explicit --tls-skip-verify=false forwarded",
			config:      "",
			args:        "--server other.example.com --token tok-other --tls-skip-verify=false",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: false,
		},
		{
			name:        "tunnel-server/tunnel-token synonyms accepted",
			config:      "",
			args:        "--tunnel-server other.example.com --tunnel-token tok-other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:        "tunnel-alias synonym accepted",
			config:      requestTestConfig,
			args:        "--tunnel-alias other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:        "tunnel-alias not in config uses the space's server",
			config:      requestTestConfig,
			args:        "--tunnel-alias unknown",
			wantServer:  "",
			wantToken:   "",
			wantSkipTLS: true,
		},
		{
			name:        "tunnel-tls-skip-verify synonym accepted",
			config:      "",
			args:        "--tunnel-server other.example.com --tunnel-token tok-other --tunnel-tls-skip-verify=false",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: false,
		},
		{
			name:        "server without token uses the space's server",
			config:      "",
			args:        "--server other.example.com",
			wantServer:  "",
			wantToken:   "",
			wantSkipTLS: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := parseTunnelCmd(t, c.config, c.args)
			request := buildStartTunnelRequest("http", 8080, "test1", cmd)

			if request.Protocol != "http" || request.Port != 8080 || request.Name != "test1" {
				t.Fatalf("base fields wrong: %+v", request)
			}
			if request.Server != c.wantServer {
				t.Fatalf("Server = %q, want %q", request.Server, c.wantServer)
			}
			if request.Token != c.wantToken {
				t.Fatalf("Token = %q, want %q", request.Token, c.wantToken)
			}
			if request.ServerTlsSkipVerify != c.wantSkipTLS {
				t.Fatalf("ServerTlsSkipVerify = %v, want %v", request.ServerTlsSkipVerify, c.wantSkipTLS)
			}
		})
	}
}
