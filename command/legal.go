package command

import (
	"context"

	"github.com/paularlott/knot/legal"

	"github.com/paularlott/cli"
)

var LegalCmd = &cli.Command{
	Name:        "legal",
	Usage:       "Show legal information",
	Description: "Output all the legal notices.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		legal.ShowLicenses()
		return nil
	},
}
