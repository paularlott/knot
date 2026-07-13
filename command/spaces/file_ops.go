package command_spaces

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
)

var ReadFileCmd = &cli.Command{
	Name:        "read-file",
	Usage:       "Read a file from a space",
	Description: "Read file contents from a running space. Use --offset and --limit to read a 1-based line range.",
	Flags: []cli.Flag{
		&cli.IntFlag{Name: "offset", Usage: "1-based line number to start at (0 = from the beginning)"},
		&cli.IntFlag{Name: "limit", Usage: "Maximum lines to return (0 = whole file)"},
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
			Usage:    "File path in the space",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		filePath := cmd.GetStringArg("path")
		offset := cmd.GetInt("offset")
		limit := cmd.GetInt("limit")

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id, false)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		var spaceId string
		for _, space := range spaces.Spaces {
			if space.Name == spaceName || space.Id == spaceName {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("Space not found: %s", spaceName)
		}

		content, totalLines, err := client.ReadSpaceFileRange(context.Background(), spaceId, filePath, offset, limit)
		if err != nil {
			return fmt.Errorf("Error reading file: %w", err)
		}

		fmt.Print(content)
		if offset > 0 || limit > 0 {
			fmt.Fprintf(os.Stderr, "%d lines (of %d total)\n", strings.Count(content, "\n")+1, totalLines)
		}
		return nil
	},
}

var WriteFileCmd = &cli.Command{
	Name:        "write-file",
	Usage:       "Write a file to a space",
	Description: "Write content to a file in a running space. Use --mode append or prepend to add to an existing file instead of overwriting.",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:    "content",
			Aliases: []string{"d"},
			Usage:   "Content to write (use - to read from stdin)",
		},
		&cli.StringFlag{
			Name:  "mode",
			Usage: "Write mode: overwrite (default), append, or prepend",
		},
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
			Usage:    "File path in the space",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		spaceName := cmd.GetStringArg("space")
		filePath := cmd.GetStringArg("path")
		content := cmd.GetString("content")

		if content == "" || content == "-" {
			bytes, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("Error reading from stdin: %w", err)
			}
			content = string(bytes)
		}

		alias := cmd.GetString("alias")
		cfg := config.GetServerAddr(alias, cmd)
		client, err := apiclient.NewClient(cfg.HttpServer, cfg.ApiToken, cmd.GetBool("tls-skip-verify"))
		if err != nil {
			return fmt.Errorf("Failed to create API client: %w", err)
		}

		user, err := client.WhoAmI(context.Background())
		if err != nil {
			return fmt.Errorf("Error getting user: %w", err)
		}

		spaces, _, err := client.GetSpaces(context.Background(), user.Id, false)
		if err != nil {
			return fmt.Errorf("Error getting spaces: %w", err)
		}

		var spaceId string
		for _, space := range spaces.Spaces {
			if space.Name == spaceName || space.Id == spaceName {
				spaceId = space.Id
				break
			}
		}

		if spaceId == "" {
			return fmt.Errorf("Space not found: %s", spaceName)
		}

		err = client.WriteSpaceFileMode(context.Background(), spaceId, filePath, content, cmd.GetString("mode"))
		if err != nil {
			return fmt.Errorf("Error writing file: %w", err)
		}

		fmt.Printf("Successfully wrote to %s\n", filePath)
		return nil
	},
}
