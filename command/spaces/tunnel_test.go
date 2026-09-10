package command_spaces

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

// parseSpaceTunnelStartCmd executes the space tunnel http command's flag
// surface against the given args and config file content, returning the
// parsed command seen by its Run function.
func parseSpaceTunnelStartCmd(t *testing.T, configFile, args string) *cli.Command {
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
				Name:        "space",
				Usage:       "Manage spaces",
				Description: "test space",
				Commands: []*cli.Command{
					{
						Name:        "tunnel",
						Usage:       "Manage a space's web tunnels",
						Description: "test tunnel",
						Commands: []*cli.Command{
							{
								Name:        "http",
								Usage:       "Start an http web tunnel in a space",
								Description: "test http",
								Arguments: []cli.Argument{
									&cli.StringArg{Name: "space", Usage: "space", Required: true},
									&cli.IntArg{Name: "port", Usage: "port", Required: true},
									&cli.StringArg{Name: "name", Usage: "name", Required: true},
								},
								MaxArgs: cli.NoArgs,
								Flags:   tunnelTargetFlags(),
								Run: func(ctx context.Context, cmd *cli.Command) error {
									captured = cmd
									return nil
								},
							},
						},
					},
				},
			},
		},
	}

	oldArgs := os.Args
	os.Args = append([]string{"knot", "space", "tunnel", "http", "myspace", "8080", "test1"}, strings.Fields(args)...)
	defer func() { os.Args = oldArgs }()

	if err := root.Execute(context.Background()); err != nil {
		t.Fatalf("failed to execute command with args %q: %v", args, err)
	}
	if captured == nil {
		t.Fatal("command did not run")
	}
	return captured
}

const tunnelTargetConfig = `
[client.connection.other]
server = "other.example.com"
token = "tok-other"
`

func TestResolveTunnelTarget(t *testing.T) {
	cases := []struct {
		name        string
		config      string
		args        string
		wantServer  string
		wantToken   string
		wantSkipTLS bool
		wantErr     bool
	}{
		{
			name:        "no flags targets the space's own server",
			config:      tunnelTargetConfig,
			args:        "",
			wantServer:  "",
			wantToken:   "",
			wantSkipTLS: true,
		},
		{
			name:        "server and token flags normalised",
			config:      "",
			args:        "--tunnel-server other.example.com --tunnel-token tok-other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:    "server without token is an error",
			config:  "",
			args:    "--tunnel-server other.example.com",
			wantErr: true,
		},
		{
			name:    "token without server is an error",
			config:  "",
			args:    "--tunnel-token tok-other",
			wantErr: true,
		},
		{
			name:        "tunnel alias resolves from config",
			config:      tunnelTargetConfig,
			args:        "--tunnel-alias other",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: true,
		},
		{
			name:    "unknown tunnel alias is an error",
			config:  tunnelTargetConfig,
			args:    "--tunnel-alias nope",
			wantErr: true,
		},
		{
			name:        "explicit --tunnel-tls-skip-verify=false",
			config:      "",
			args:        "--tunnel-server other.example.com --tunnel-token tok-other --tunnel-tls-skip-verify=false",
			wantServer:  "https://other.example.com",
			wantToken:   "tok-other",
			wantSkipTLS: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := parseSpaceTunnelStartCmd(t, c.config, c.args)
			server, token, skipVerify, err := resolveTunnelTarget(cmd)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected an error, got server=%q token=%q", server, token)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if server != c.wantServer || token != c.wantToken || skipVerify != c.wantSkipTLS {
				t.Fatalf("got (%q, %q, %v), want (%q, %q, %v)", server, token, skipVerify, c.wantServer, c.wantToken, c.wantSkipTLS)
			}
		})
	}
}
