//go:build server

package main

import (
	"github.com/paularlott/cli"
)

// applyDesktopMode is a no-op in -tags server builds: bare `knot` shows help
// and desktop-mode code is excluded from the binary entirely.
func applyDesktopMode(cmd *cli.Command) {}
