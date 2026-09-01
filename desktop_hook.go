//go:build !server

package main

import (
	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/desktop"
)

// applyDesktopMode makes bare `knot` (no subcommand) run the server with a
// system tray icon instead of printing help.
func applyDesktopMode(cmd *cli.Command) {
	cmd.Run = desktop.Run
}
