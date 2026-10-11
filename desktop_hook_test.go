//go:build !server

package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/paularlott/cli"
)

// An unknown subcommand must be an error, not a reason to start desktop mode.
func TestDesktopModeRejectsUnknownCommand(t *testing.T) {
	saved := os.Args
	t.Cleanup(func() { os.Args = saved })
	for _, args := range [][]string{{"knot", "nosuch"}, {"knot", "space list"}} {
		os.Args = args
		cmd := &cli.Command{
			Name:     "knot",
			Commands: []*cli.Command{{Name: "ping", Run: func(ctx context.Context, cmd *cli.Command) error { return nil }}},
		}
		applyDesktopMode(cmd)
		err := cmd.Execute(context.Background())
		if err == nil || !strings.Contains(err.Error(), "unknown command") {
			t.Errorf("knot %q: got %v, want an unknown command error", args[1:], err)
		}
	}
}
