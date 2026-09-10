package cmdutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/paularlott/cli"
	cli_toml "github.com/paularlott/cli/toml"
)

// parseTunnelLikeCmd executes a command carrying the tunnel command's
// connection flags against the given args and config file content, and
// returns the parsed command seen by its Run function.
func parseTunnelLikeCmd(t *testing.T, configFile, args string) *cli.Command {
	t.Helper()

	path := filepath.Join(t.TempDir(), "knot.toml")
	if configFile != "" {
		if err := os.WriteFile(path, []byte(configFile), 0600); err != nil {
			t.Fatalf("failed to write config file: %v", err)
		}
	}

	var captured *cli.Command
	cmd := &cli.Command{
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
						Flags: []cli.Flag{
							&cli.StringFlag{Name: "server", Aliases: []string{"s"}, EnvVars: []string{"KNOT_SERVER"}},
							&cli.StringFlag{Name: "token", Aliases: []string{"t"}, EnvVars: []string{"KNOT_TOKEN"}},
							&cli.StringFlag{Name: "alias", Aliases: []string{"a"}, DefaultValue: "default"},
						},
						Run: func(ctx context.Context, cmd *cli.Command) error {
							captured = cmd
							return nil
						},
					},
				},
			},
		},
	}

	oldArgs := os.Args
	os.Args = append([]string{"knot", "tunnel", "http"}, splitArgs(args)...)
	defer func() { os.Args = oldArgs }()

	if err := cmd.Execute(context.Background()); err != nil {
		t.Fatalf("failed to execute command with args %q: %v", args, err)
	}
	if captured == nil {
		t.Fatal("command did not run")
	}
	return captured
}

func splitArgs(args string) []string {
	if args == "" {
		return nil
	}
	return strings.Fields(args)
}

const testConfig = `
[client.connection.default]
server = "https://owning.example.com"
token = "tok-owning"

[client.connection.other]
server = "other.example.com"
token = "tok-other"

[client.connection.half]
server = "https://half.example.com"
`

func TestExplicitServerAddr(t *testing.T) {
	cases := []struct {
		name       string
		configFile string
		args       string
		want       string // expected HttpServer, empty means nil expected
	}{
		{
			name:       "no flags no alias falls back even with config",
			configFile: testConfig,
			args:       "",
			want:       "",
		},
		{
			name:       "server and token flags",
			configFile: "",
			args:       "--server remote.example.com --token tok-remote",
			want:       "https://remote.example.com",
		},
		{
			name:       "server flag only is not explicit",
			configFile: "",
			args:       "--server remote.example.com",
			want:       "",
		},
		{
			name:       "token flag only is not explicit",
			configFile: "",
			args:       "--token tok-remote",
			want:       "",
		},
		{
			name:       "explicit alias resolving in config",
			configFile: testConfig,
			args:       "-a other",
			want:       "https://other.example.com",
		},
		{
			name:       "explicit alias not in config falls back",
			configFile: testConfig,
			args:       "-a unknown",
			want:       "",
		},
		{
			name:       "alias missing token falls back",
			configFile: testConfig,
			args:       "-a half",
			want:       "",
		},
		{
			name:       "default alias only counts when explicitly given",
			configFile: testConfig,
			args:       "-a default",
			want:       "https://owning.example.com",
		},
		{
			name:       "flags win over alias",
			configFile: testConfig,
			args:       "-a other --server flags.example.com --token tok-flags",
			want:       "https://flags.example.com",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := parseTunnelLikeCmd(t, c.configFile, c.args)
			addr := ExplicitServerAddr(cmd)
			if c.want == "" {
				if addr != nil {
					t.Fatalf("expected nil, got %+v", addr)
				}
				return
			}
			if addr == nil {
				t.Fatal("expected an address, got nil")
			}
			if addr.HttpServer != c.want {
				t.Fatalf("HttpServer = %q, want %q", addr.HttpServer, c.want)
			}
		})
	}
}

// Spaces inject KNOT_SERVER pointing at the owning server; a token in the
// environment would then make the pair count as explicit. Verify env vars do
// drive resolution, since the tunnel flags carry those EnvVars.
func TestExplicitServerAddrFromEnv(t *testing.T) {
	t.Setenv("KNOT_SERVER", "env.example.com")
	t.Setenv("KNOT_TOKEN", "tok-env")

	cmd := parseTunnelLikeCmd(t, "", "")
	addr := ExplicitServerAddr(cmd)
	if addr == nil {
		t.Fatal("expected env-provided server/token to resolve")
	}
	if addr.HttpServer != "https://env.example.com" || addr.ApiToken != "tok-env" {
		t.Fatalf("unexpected addr: %+v", addr)
	}
}
