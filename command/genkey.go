package command

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/paularlott/cli"
)

var GenkeyCmd = &cli.Command{
	Name:        "genkey",
	Usage:       "Generate Encryption Key",
	Description: "Generate an encryption key for encrypting stored variables.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		key := crypt.CreateKey()
		fmt.Println("Encryption Key:", key)
		fmt.Println("")
		return nil
	},
}
