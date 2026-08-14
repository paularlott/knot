//go:build !server

// Package desktop implements knot's desktop mode: bare `knot` runs the full
// server (identical to `knot server`) plus a system tray icon whose menu
// offers to open the web UI in the default browser and to quit. The whole
// package is excluded from -tags server builds.
package desktop

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/log"
)

// Monochrome menu-bar icons generated from web/public_html/images/logo.svg
// (see the `task icons` build task).
var (
	//go:embed icon.png
	icon []byte
	//go:embed icon-white.png
	iconWhite []byte
)

// Run starts the knot server in a background goroutine and the system tray on
// the calling (main) goroutine, blocking until the user quits from the tray
// menu. If the tray is unavailable the server keeps running headless and
// Ctrl-C still shuts it down via the signal path in RunServer.
func Run(ctx context.Context, cmd *cli.Command) error {
	logger := log.WithGroup("desktop")

	shutdown := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- command.RunServer(cmd, shutdown)
	}()

	url := cmd.GetString("url")

	// If the server exits on its own (e.g. Ctrl-C), tear the tray down so
	// the process exits instead of idling with a dead server.
	t := newTray(url, func() { close(shutdown) })
	defer t.stop()
	go func() {
		err := <-done
		t.stop()

		// The macOS message loop doesn't always return from Run() after
		// Remove(); the server cleanup has already completed at this
		// point, so give the loop a brief grace period and then exit
		// with the server's exit status.
		time.Sleep(500 * time.Millisecond)
		if err != nil {
			logger.Error("server exited", "error", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

	if err := t.run(); err != nil {
		logger.Error("system tray unavailable, running headless", "error", err)
		fmt.Fprintf(os.Stderr, "Open %s in your browser, press Ctrl-C to stop.\n", url)
		return <-done
	}

	return <-done
}
