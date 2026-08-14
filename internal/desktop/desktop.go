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
	neturl "net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/configwizard"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util"
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

	// On Windows a bare `knot` is GUI usage: restart with no console at
	// all and exit, so no terminal window accompanies the tray. All
	// subcommands keep their normal console behavior.
	if util.RelaunchHidden() {
		return nil
	}
	util.HideConsoleIfOwned()

	url := cmd.GetString("url")

	// First-run setup: if there is no config file, or it configures no
	// database backend, serve the setup wizard instead of the server.
	if needsSetup(cmd) {
		return runSetup(ctx, cmd, url)
	}

	shutdown := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		done <- command.RunServer(cmd, shutdown)
	}()

	// If the server exits on its own (e.g. Ctrl-C), tear the tray down so
	// the process exits instead of idling with a dead server.
	t := newTray(url, "Open knot UI", url+"/setup", func() { close(shutdown) })
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

// needsSetup reports whether the setup wizard should run: only when no
// configuration file was found. The wizard writes a complete config
// (required fields are validated on save), and the desktop-mode fallbacks
// in buildServerConfig fill any remaining gaps such as a missing database.
func needsSetup(cmd *cli.Command) bool {
	logger := log.WithGroup("desktop")

	fileUsed := ""
	if cmd.ConfigFile != nil {
		fileUsed = cmd.ConfigFile.FileUsed()
	}
	logger.Debug("setup check", "config_file", fileUsed)

	return fileUsed == ""
}

// runSetup serves the config wizard on the advertised address, pre-filled
// from any existing config and writing to ~/.knot/knot.toml. After the user
// saves, the app keeps running and asks to be restarted so the new config is
// loaded cleanly.
func runSetup(ctx context.Context, cmd *cli.Command, url string) error {
	logger := log.WithGroup("desktop")

	// The wizard always binds loopback — the configured URL may name a
	// public address that does not resolve yet; only its port is reused.
	// The tray opens the wizard's real address, not the configured URL.
	addr := "127.0.0.1:3000"
	if u, err := neturl.Parse(url); err == nil && u.Port() != "" {
		addr = "127.0.0.1:" + u.Port()
	}
	wizardURL := "http://" + addr + "/"

	target := ""
	if home, err := os.UserHomeDir(); err == nil {
		target = filepath.Join(home, "."+config.CONFIG_DIR, config.CONFIG_FILE)
	}

	wizardDone := make(chan error, 1)
	go func() {
		wizardDone <- configwizard.Serve(ctx, addr, cmd.GetString("config"), configwizard.Options{
			TargetPath:     target,
			AllowOverwrite: true,
			Desktop:        true,
		})
	}()

	t := newTray(wizardURL, "Open knot setup", "", func() {})
	defer t.stop()
	go func() {
		if err := <-wizardDone; err != nil {
			logger.Error("setup wizard exited", "error", err)
			return
		}
		logger.Info("setup complete, restart knot to apply the new configuration")
		t.notify("knot setup complete", "Quit knot from the tray menu and reopen it to apply the new configuration.")
	}()

	if err := t.run(); err != nil {
		logger.Error("system tray unavailable, running headless", "error", err)
		fmt.Fprintf(os.Stderr, "Open %s in your browser to complete the setup, then restart knot.\n", wizardURL)
		return <-wizardDone
	}

	return nil
}
