package command_ssh_config

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/util"

	"github.com/paularlott/cli"
)

var SshConfigRemoveCmd = &cli.Command{
	Name:        "remove",
	Usage:       "Remove all entries from .ssh/config",
	Description: "Remove any knot space configurations from the .ssh/config file.",
	MaxArgs:     cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		alias := cmd.GetString("alias")
		util.UpdateSSHConfig("", alias)
		fmt.Println(".ssh/config has been updated")
		return nil
	},
}
