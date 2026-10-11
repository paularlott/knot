//go:build !server

package main

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/desktop"
)

// applyDesktopMode makes bare `knot` (no subcommand) run the server with a
// system tray icon instead of printing help.
func applyDesktopMode(cmd *cli.Command) {
	cmd.Run = desktopRun
}

// desktopRun starts desktop mode only for a bare `knot`. With a root Run the
// cli package hands an unknown subcommand to it as an argument, which must
// fail rather than start the server or the setup wizard.
func desktopRun(ctx context.Context, cmd *cli.Command) error {
	if args := cmd.GetArgs(); len(args) > 0 {
		return fmt.Errorf("unknown command %q\n\nRun '%s --help' for usage.", args[0], cmd.Name)
	}
	return desktop.Run(ctx, cmd)
}
