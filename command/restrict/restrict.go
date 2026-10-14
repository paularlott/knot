// Package command_restrict holds the commands that restrict a space and
// show its restrictions, shared by the knot command (knot space restrict)
// and the agent inside a space (knot agent restrict).
package command_restrict

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/agentlink"
)

const restrictDescription = "Any restriction also confines the space: it can then act only on itself, the buckets it's allowed, tunnels and port forwards, and start or stop other spaces. " +
	"Repeat --bucket to keep only the buckets listed, adding :ro to make one read-only. " +
	"--no-cli locks the space's processes out of knot entirely: only restarting or shutting the space down still works from inside, while port forwards and scripts started from outside the space keep working. "

// RestrictCmd restricts a space. With spaceArg the space can be named, and
// defaults to the space it runs in; without, it's always that space.
func RestrictCmd(spaceArg bool) *cli.Command {
	cmd := &cli.Command{
		Name:    "restrict",
		MaxArgs: cli.NoArgs,
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:  "bucket",
				Usage: "Keep only this bucket, as name or name:ro (can be specified multiple times).",
			},
			&cli.StringFlag{
				Name:  "files",
				Usage: "File storage for every bucket: ro (read-only) or none.",
			},
			&cli.BoolFlag{
				Name:  "no-files",
				Usage: "Switch file storage off, the same as --files none.",
			},
			&cli.BoolFlag{
				Name:  "no-secrets",
				Usage: "Switch personal secrets off.",
			},
			&cli.BoolFlag{
				Name:  "no-cli",
				Usage: "Lock the space's processes out of knot, except restarting or shutting the space down.",
			},
			&cli.BoolFlag{Name: "json", Usage: "Print the restrictions as JSON"},
		},
		Run: func(ctx context.Context, cmd *cli.Command) error {
			space, err := targetSpace(cmd, spaceArg)
			if err != nil {
				return err
			}
			client, err := cmdutil.GetClient(cmd)
			if err != nil {
				return err
			}

			var res *apiclient.SpaceRestrictions
			if spaceArg && cmd.GetBool("clear") {
				if len(cmd.GetStringSlice("bucket")) > 0 || cmd.GetString("files") != "" || cmd.GetBool("no-files") || cmd.GetBool("no-secrets") || cmd.GetBool("no-cli") {
					return fmt.Errorf("--clear can't be used with other restrictions")
				}
				res, _, err = client.ClearSpaceRestrictions(ctx, space)
				if err != nil {
					return fmt.Errorf("couldn't lift the restrictions on space %q: %w", space, cmdutil.CleanErr(err))
				}
			} else {
				request, err := restrictRequest(cmd)
				if err != nil {
					return err
				}
				res, _, err = client.RestrictSpace(ctx, space, request)
				if err != nil {
					return fmt.Errorf("couldn't restrict space %q: %w", space, cmdutil.CleanErr(err))
				}
			}

			if cmd.GetBool("json") {
				return printJSON(res)
			}
			printRestrictions(res)
			return nil
		},
	}

	if spaceArg {
		cmd.Usage = "Restrict what a space can reach"
		cmd.Description = "Restrict what a space's own credential can do, until the space restarts. Run inside a space, it restricts that space; elsewhere, name the space. " +
			"From inside a space restrictions only ever tighten, so a startup script can lock a space down before anything else in it runs. " +
			restrictDescription + "--clear lifts every restriction, and works only from outside the space."
		cmd.Arguments = []cli.Argument{
			&cli.StringArg{Name: "space", Usage: "The space to restrict, default the space this runs in"},
		}
		cmd.Flags = append(cmd.Flags, &cli.BoolFlag{
			Name:  "clear",
			Usage: "Lift every restriction (not from inside the space).",
		})
	} else {
		cmd.Usage = "Restrict what this space can reach"
		cmd.Description = "Restrict what this space's own credential can do, until the space restarts. Restrictions only ever tighten, so a startup script can lock the space down before anything else in it runs. " +
			restrictDescription + "Only restarting the space, or its owner from outside it, lifts them."
	}
	return cmd
}

// RestrictionsCmd shows a space's restrictions, as RestrictCmd names it.
func RestrictionsCmd(spaceArg bool) *cli.Command {
	cmd := &cli.Command{
		Name:    "restrictions",
		MaxArgs: cli.NoArgs,
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "Print the restrictions as JSON"},
		},
		Run: func(ctx context.Context, cmd *cli.Command) error {
			space, err := targetSpace(cmd, spaceArg)
			if err != nil {
				return err
			}
			client, err := cmdutil.GetClient(cmd)
			if err != nil {
				return err
			}
			res, _, err := client.GetSpaceRestrictions(ctx, space)
			if err != nil {
				return fmt.Errorf("couldn't get the restrictions on space %q: %w", space, cmdutil.CleanErr(err))
			}
			if cmd.GetBool("json") {
				return printJSON(res)
			}
			printRestrictions(res)
			return nil
		},
	}
	if spaceArg {
		cmd.Usage = "Show what a space is restricted to"
		cmd.Description = "Show a space's restrictions, including what its template switches off. Run inside a space, it shows that space's."
		cmd.Arguments = []cli.Argument{
			&cli.StringArg{Name: "space", Usage: "The space to show, default the space this runs in"},
		}
	} else {
		cmd.Usage = "Show what this space is restricted to"
		cmd.Description = "Show this space's restrictions, including what its template switches off."
	}
	return cmd
}

// targetSpace is the space named on the command line or, inside a space,
// the space itself.
func targetSpace(cmd *cli.Command, spaceArg bool) (string, error) {
	if spaceArg {
		if space := cmd.GetStringArg("space"); space != "" {
			return space, nil
		}
	}
	if agentlink.IsAgentRunning() {
		id, err := agentlink.GetSpaceId()
		if err != nil {
			return "", fmt.Errorf("couldn't find which space this is: %w", err)
		}
		return id, nil
	}
	if spaceArg {
		return "", fmt.Errorf("name the space, or run this inside one")
	}
	return "", fmt.Errorf("this only works inside a space")
}

func restrictRequest(cmd *cli.Command) (*apiclient.SpaceRestrictRequest, error) {
	request := &apiclient.SpaceRestrictRequest{
		NoSecrets: cmd.GetBool("no-secrets"),
		NoCli:     cmd.GetBool("no-cli"),
		Files:     strings.ToLower(strings.TrimSpace(cmd.GetString("files"))),
	}
	switch request.Files {
	case "", "ro", "none":
	default:
		return nil, fmt.Errorf("--files takes ro or none, not %q", request.Files)
	}
	if cmd.GetBool("no-files") {
		request.Files = "none"
	}
	for _, b := range cmd.GetStringSlice("bucket") {
		name, mode, _ := strings.Cut(strings.TrimSpace(b), ":")
		if name == "" {
			return nil, fmt.Errorf("--bucket needs a bucket name")
		}
		switch mode {
		case "", "rw":
			request.Buckets = append(request.Buckets, apiclient.SpaceRestrictBucket{Name: name})
		case "ro":
			request.Buckets = append(request.Buckets, apiclient.SpaceRestrictBucket{Name: name, ReadOnly: true})
		default:
			return nil, fmt.Errorf("--bucket %s: add :ro for read-only, or nothing", b)
		}
	}
	return request, nil
}

func printRestrictions(res *apiclient.SpaceRestrictions) {
	fmt.Printf("Space:       %s\n", res.SpaceName)
	if !res.Restricted {
		fmt.Println("Restricted:  no")
	} else {
		fmt.Println("Restricted:  yes, until the space restarts")
	}

	secrets := "allowed"
	switch {
	case res.TemplateNoSecrets:
		secrets = "off (template)"
	case res.NoSecrets:
		secrets = "off"
	}
	fmt.Printf("Secrets:     %s\n", secrets)
	if res.NoCli {
		fmt.Println("CLI:         restart and shut down only")
	}

	files := "all buckets"
	switch {
	case res.TemplateNoFiles:
		files = "off (template)"
	case res.Files == "none":
		files = "off"
	case res.LimitBuckets && len(res.Buckets) == 0:
		files = "no buckets"
	case res.LimitBuckets:
		files = "only the buckets below"
	}
	if res.Files == "ro" && !res.TemplateNoFiles {
		files += ", read-only"
	}
	fmt.Printf("Files:       %s\n", files)

	if res.LimitBuckets && res.Files != "none" && !res.TemplateNoFiles {
		for _, b := range res.Buckets {
			if b.ReadOnly || res.Files == "ro" {
				fmt.Printf("  %s (read-only)\n", b.Name)
			} else {
				fmt.Printf("  %s\n", b.Name)
			}
		}
	}
}

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
