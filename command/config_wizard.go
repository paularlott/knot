package command

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/internal/configwizard"

	"github.com/paularlott/cli"
)

var ConfigWizardCmd = &cli.Command{
	Name:  "config-wizard",
	Usage: "Run the web-based config wizard",
	Description: `Start a local web UI that generates a knot.toml in an embedded TOML editor.

The wizard always starts. When no config exists yet, the editor's Write button
saves straight to disk; when a config already exists, Write is disabled and the
generated text is shown for manual copy / merge.

Listens on 127.0.0.1 by default; bind a different address with --listen. A
one-time bootstrap token is printed to stderr and must be supplied to access the
UI, so the wizard cannot be driven by someone who cannot already read your
terminal. Use --config to target a non-default output path.`,
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:         "port",
			Usage:        "TCP port to serve the wizard web UI on.",
			DefaultValue: 8080,
		},
		&cli.StringFlag{
			Name:         "listen",
			Usage:        "Address to bind the wizard web UI to. Defaults to loopback.",
			DefaultValue: "127.0.0.1",
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		addr := fmt.Sprintf("%s:%d", cmd.GetString("listen"), cmd.GetInt("port"))

		sigCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
		defer stop()

		return configwizard.Serve(sigCtx, addr, cmd.GetString("config"))
	},
}
