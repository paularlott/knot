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

const daemonTestConfig = `
[client.connection.other]
server = "other.example.com"
token = "tok-other"
`

func TestDaemonRemoteTargetErr(t *testing.T) {
	cases := []struct {
		name      string
		config    string
		args      string
		wantError bool
	}{
		{name: "no explicit target", config: daemonTestConfig, args: "", wantError: false},
		{name: "explicit server and token", config: "", args: "--server other.example.com --token tok", wantError: true},
		{name: "explicit alias resolving in config", config: daemonTestConfig, args: "-a other", wantError: true},
		{name: "explicit alias not in config", config: daemonTestConfig, args: "-a unknown", wantError: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := parseTunnelCmd(t, c.config, c.args)
			err := daemonRemoteTargetErr(cmd)
			if c.wantError && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !c.wantError && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
