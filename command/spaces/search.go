package command_spaces

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

func resolveSpaceID(ctx context.Context, client *apiclient.ApiClient, nameOrID string) (string, error) {
	space, err := client.GetSpaceByName(ctx, nameOrID)
	if err != nil {
		return "", fmt.Errorf("error getting space: %w", err)
	}
	return space.SpaceId, nil
}

// GrepCmd searches file contents in a space.
var GrepCmd = &cli.Command{
	Name:        "grep",
	Usage:       "Search file contents in a space",
	Description: "Search for a pattern in files inside a running space. Output is one match per line as 'file:line: text'. Use --json for structured output.",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "ignore-case", Aliases: []string{"i"}, Usage: "Case-insensitive matching"},
		&cli.BoolFlag{Name: "literal", Usage: "Treat PATTERN as a literal string, not a regex"},
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Recurse into subdirectories"},
		&cli.StringFlag{Name: "glob", Usage: "Only search files matching this glob, e.g. '*.py'"},
		&cli.BoolFlag{Name: "json", Usage: "Emit the raw structured result as JSON"},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "space", Required: true, Usage: "Name or ID of the space"},
		&cli.StringArg{Name: "pattern", Required: true, Usage: "Regular expression (or literal with --literal)"},
		&cli.StringArg{Name: "path", Required: false, Usage: "File or directory to search (default: current directory)"},
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

		path := cmd.GetStringArg("path")
		if path == "" {
			path = "."
		}

		result, err := client.Grep(ctx, spaceId, apiclient.GrepRequest{
			Pattern:    cmd.GetStringArg("pattern"),
			Path:       path,
			Literal:    cmd.GetBool("literal"),
			Recursive:  cmd.GetBool("recursive"),
			IgnoreCase: cmd.GetBool("ignore-case"),
			Glob:       cmd.GetString("glob"),
		})
		if err != nil {
			return err
		}
		if cmd.GetBool("json") {
			return printJSON(result)
		}
		for _, m := range result.Matches {
			fmt.Printf("%s:%d: %s\n", m.File, m.Line, m.Text)
		}
		return nil
	},
}

// FindCmd finds files/directories in a space.
var FindCmd = &cli.Command{
	Name:        "find",
	Usage:       "Find files in a space",
	Description: "Find files and directories in a running space by name, type, or size. Output is one path per line by default; --long adds size, mtime, and type. Recursive by default.",
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "name", Aliases: []string{"n"}, Usage: "Shell-style glob matched against the base name, e.g. '*.md'"},
		&cli.StringFlag{Name: "type", Aliases: []string{"t"}, Usage: "Restrict to 'file', 'dir', or 'any' (default 'any')"},
		&cli.BoolFlag{Name: "recursive", DefaultValue: true, Usage: "Descend into subdirectories (default true)"},
		&cli.BoolFlag{Name: "include-hidden", Usage: "Match entries whose name starts with '.'"},
		&cli.IntFlag{Name: "max-depth", Usage: "Maximum recursion depth (0 = unlimited)"},
		&cli.IntFlag{Name: "size-min", Usage: "Minimum size in bytes"},
		&cli.IntFlag{Name: "size-max", Usage: "Maximum size in bytes"},
		&cli.BoolFlag{Name: "long", Aliases: []string{"l"}, Usage: "List size, mtime, and type alongside each path"},
		&cli.BoolFlag{Name: "json", Usage: "Emit the raw structured result as JSON"},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "space", Required: true, Usage: "Name or ID of the space"},
		&cli.StringArg{Name: "path", Required: false, Usage: "Directory to search under (default: current directory)"},
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

		path := cmd.GetStringArg("path")
		if path == "" {
			path = "."
		}

		long := cmd.GetBool("long")
		req := apiclient.FindRequest{
			Path:            path,
			Recursive:       cmd.GetBool("recursive"),
			Type:            cmd.GetString("type"),
			Name:            cmd.GetString("name"),
			IncludeHidden:   cmd.GetBool("include-hidden"),
			IncludeMetadata: long,
			MaxDepth:        cmd.GetInt("max-depth"),
		}
		if n := int64(cmd.GetInt("size-min")); n != 0 {
			req.SizeMin = &n
		}
		if n := int64(cmd.GetInt("size-max")); n != 0 {
			req.SizeMax = &n
		}

		result, err := client.Find(ctx, spaceId, req)
		if err != nil {
			return err
		}
		if cmd.GetBool("json") {
			return printJSON(result)
		}
		if long {
			for _, e := range result.Entries {
				kind := "f"
				if e.IsDir {
					kind = "d"
				}
				fmt.Printf("%s %12d %s %s\n", kind, e.Size, time.Unix(0, int64(e.Mtime*1e9)).UTC().Format("2006-01-02 15:04:05"), e.Path)
			}
			return nil
		}
		for _, p := range result.Paths {
			fmt.Println(p)
		}
		return nil
	},
}

// SedCmd performs in-place edits or capture extraction in a space.
var SedCmd = &cli.Command{
	Name:        "sed",
	Usage:       "In-place edit or extract from files in a space",
	Description: "By default performs a literal string replacement (s/OLD/NEW/). Pass --regex to treat OLD as a regular expression (capture groups ${1}, ${name} allowed in NEW). Pass --extract to return capture groups of a regex instead of modifying files.",
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "regex", Usage: "Treat OLD/PATTERN as a regular expression"},
		&cli.BoolFlag{Name: "extract", Usage: "Extract capture groups from PATTERN (no modification); NEW is ignored"},
		&cli.BoolFlag{Name: "ignore-case", Aliases: []string{"i"}, Usage: "Case-insensitive matching"},
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Recurse into subdirectories when PATH is a directory"},
		&cli.StringFlag{Name: "glob", Usage: "Only touch files matching this glob, e.g. '*.py'"},
		&cli.BoolFlag{Name: "json", Usage: "Emit the raw structured result as JSON"},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "space", Required: true, Usage: "Name or ID of the space"},
		&cli.StringArg{Name: "old-or-pattern", Required: true, Usage: "Literal string to replace (or regex with --regex/--extract)"},
		&cli.StringArg{Name: "new", Required: false, Usage: "Replacement string (required unless --extract)"},
		&cli.StringArg{Name: "path", Required: false, Usage: "File or directory (default: current directory)"},
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

		oldOrPattern := cmd.GetStringArg("old-or-pattern")
		new := cmd.GetStringArg("new")
		path := cmd.GetStringArg("path")
		if path == "" {
			path = "."
		}

		req := apiclient.SedRequest{
			Pattern:    oldOrPattern,
			Path:       path,
			Recursive:  cmd.GetBool("recursive"),
			IgnoreCase: cmd.GetBool("ignore-case"),
			Glob:       cmd.GetString("glob"),
		}

		switch {
		case cmd.GetBool("extract"):
			req.Mode = "extract"
		case cmd.GetBool("regex"):
			if new == "" {
				return fmt.Errorf("--regex requires a NEW replacement argument")
			}
			req.Mode = "replace_pattern"
			req.Replacement = new
		default:
			if new == "" {
				return fmt.Errorf("sed replace requires OLD and NEW arguments")
			}
			req.Mode = "replace"
			req.Replacement = new
		}

		result, err := client.Sed(ctx, spaceId, req)
		if err != nil {
			return err
		}
		if cmd.GetBool("json") {
			return printJSON(result)
		}
		if req.Mode == "extract" {
			for _, m := range result.Matches {
				fmt.Printf("%s:%d: %s\n", m.File, m.Line, m.Text)
				if len(m.Groups) > 0 {
					fmt.Printf("  groups: %v\n", m.Groups)
				}
			}
		} else {
			fmt.Printf("%s file(s) modified\n", strconv.FormatInt(result.FilesModified, 10))
		}
		return nil
	},
}

func printJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
