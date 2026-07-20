package command_spaces

import (
	"context"
	"fmt"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

// DeleteFileCmd removes a file or directory from a running space.
var DeleteFileCmd = &cli.Command{
	Name:        "delete-file",
	Usage:       "Delete a file or directory in a space",
	Description: "Delete a file or directory in a running space. Use --recursive to remove a non-empty directory. Missing paths are treated as success (idempotent).",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Recursively remove a directory and its contents"},
		&cli.StringFlag{Name: "workdir", Aliases: []string{"w"}, Usage: "Working directory for relative paths in space", DefaultValue: ""},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "space",
			Required: true,
			Usage:    "Name or ID of the space",
		},
		&cli.StringArg{
			Name:     "path",
			Required: true,
			Usage:    "File or directory path in the space",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}
		spaceId, err := resolveSpaceID(ctx, client, cmd.GetStringArg("space"))
		if err != nil {
			return err
		}

		req := apiclient.DeleteFileRequest{
			Path:      cmd.GetStringArg("path"),
			Recursive: cmd.GetBool("recursive"),
			Workdir:   cmd.GetString("workdir"),
		}

		result, err := client.DeleteSpaceFile(ctx, spaceId, req)
		if err != nil {
			return err
		}
		if result.Removed > 0 {
			fmt.Printf("Removed %d entr%s\n", result.Removed, pluralize(result.Removed))
		} else {
			fmt.Println("Path did not exist (no-op)")
		}
		return nil
	},
}

// pluralize returns "y"/"ies" for the Removed counter so output reads naturally.
func pluralize(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}
