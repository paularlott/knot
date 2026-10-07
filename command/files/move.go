package command_files

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
)

var moveCmd = &cli.Command{
	Name:    "move",
	Aliases: []string{"mv"},
	Usage:   "Move or rename files and folders in buckets",
	Description: `Move or rename bucket files, giving each as bucket:path.

Rename: knot file mv b1:old.txt b1:new.txt

Move into a folder, keeping the name: knot file mv b1:logs/app.log b1:archive/

Several sources go into a destination folder, which is a bucket path ending in / (or an existing folder). A folder moves with everything below it. A source may hold wildcards, quoted: 'b1:logs/*.log'.

Within a bucket the move happens on the server, and no content is transferred. Between buckets the files are copied on the server and then removed from the source. A move that would replace an existing file is refused unless --overwrite is given.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "overwrite", Aliases: []string{"f"}, Usage: "Replace files that already exist at the destination."},
	},
	MinArgs: 2,
	MaxArgs: cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return runMove(ctx, client, cmd.GetArgs(), cmd.GetBool("overwrite"))
	},
}

// moveUnit is one file, or one folder and everything below it, to move.
type moveUnit struct {
	bucket     string // as the user wrote it
	full       string // the bucket's full name
	from, to   string // keys, with no trailing slash
	folder     bool
	display    string
	toDisplay  string
	destBucket string // as the user wrote it
	destFull   string
}

// runMove moves the files named by args, the last being the destination.
func runMove(ctx context.Context, client *apiclient.ApiClient, args []string, overwrite bool) error {
	if len(args) < 2 {
		return fmt.Errorf("move needs a source and a destination, e.g. knot file mv b1:old.txt b1:new.txt")
	}
	dst, err := requireRemote(args[len(args)-1])
	if err != nil {
		return err
	}

	names := map[string]string{}
	fullName := func(ref string) (string, error) {
		if n, ok := names[ref]; ok {
			return n, nil
		}
		b, err := client.GetFileBucket(ctx, ref)
		if err != nil {
			return "", apiError(err)
		}
		names[ref] = b.Name
		return b.Name, nil
	}
	dstFull, err := fullName(dst.bucket)
	if err != nil {
		return err
	}

	// A destination that ends in a slash, or names an existing folder, takes
	// the sources into it; otherwise it is the new name.
	destFolder := isFolderKey(dst.key)
	if !destFolder {
		info, err := statRemote(ctx, client, dst.bucket, dst.key)
		if err != nil {
			return err
		}
		destFolder = info.folder && info.file == nil
	}
	destPrefix := ""
	if destFolder {
		destPrefix = dirPrefix(dst.key)
	}
	destName := strings.TrimSuffix(dst.key, "/")

	srcArgs := args[:len(args)-1]
	if len(srcArgs) > 1 && !destFolder {
		return fmt.Errorf("%s is not a folder: end it with / to move several things into it", dst)
	}

	var units []moveUnit
	for _, a := range srcArgs {
		loc, err := requireRemote(a)
		if err != nil {
			return err
		}
		if loc.key == "" {
			return fmt.Errorf("%s is a whole bucket and cannot be moved", loc)
		}
		full, err := fullName(loc.bucket)
		if err != nil {
			return err
		}
		unit := func(from, to string, folder bool) moveUnit {
			return moveUnit{bucket: loc.bucket, full: full, from: from, to: to, folder: folder,
				display: remoteName(loc.bucket, from), toDisplay: remoteName(dst.bucket, to), destBucket: dst.bucket, destFull: dstFull}
		}

		if hasGlob(loc.key) {
			files, _, err := remoteSources(ctx, client, loc, false, false)
			if err != nil {
				return err
			}
			if !destFolder && len(files) > 1 {
				return fmt.Errorf("%s matches %d files: the destination must be a folder, ending in /", loc, len(files))
			}
			for _, f := range files {
				to := destPrefix + f.rel
				if !destFolder {
					to = destName
				}
				units = append(units, unit(f.key, to, false))
			}
			continue
		}

		info, err := statRemote(ctx, client, loc.bucket, loc.key)
		if err != nil {
			return err
		}
		if info.file == nil && !info.folder {
			return fmt.Errorf("%s: no such file or folder", loc)
		}
		from := strings.TrimSuffix(loc.key, "/")
		to := destName
		if destFolder {
			to = destPrefix + path.Base(from)
		}
		if to == "" {
			return fmt.Errorf("no destination name for %s", loc)
		}
		units = append(units, unit(from, to, info.folder && (info.file == nil || isFolderKey(loc.key))))
	}

	// Refuse before moving anything.
	for _, u := range units {
		if u.full == u.destFull && (u.to == u.from || strings.HasPrefix(u.to, u.from+"/")) {
			if u.to == u.from {
				return fmt.Errorf("%s and %s are the same", u.display, u.toDisplay)
			}
			return fmt.Errorf("cannot move %s into itself", u.display)
		}
	}

	total := 0
	for _, u := range units {
		var n int
		if u.full == u.destFull {
			n, err = client.MoveFileObjects(ctx, apiclient.FileMoveRequest{Bucket: u.bucket, From: u.from, To: u.to, Overwrite: overwrite})
			if err != nil {
				return fmt.Errorf("%s: %s", u.display, apiError(err))
			}
		} else if n, err = moveAcross(ctx, client, u, overwrite); err != nil {
			return fmt.Errorf("%s: %w", u.display, err)
		}
		fmt.Printf("moved %s -> %s\n", u.display, u.toDisplay)
		total += n
	}
	if len(units) > 1 || total > 1 {
		fmt.Printf("%d files moved\n", total)
	}
	return nil
}

// moveAcross moves a file or folder to another bucket: every file is copied
// on the server first, and the sources are removed only once all have been.
func moveAcross(ctx context.Context, client *apiclient.ApiClient, u moveUnit, overwrite bool) (int, error) {
	var keys []string
	if info, err := statRemote(ctx, client, u.bucket, u.from); err != nil {
		return 0, err
	} else if info.file != nil {
		keys = append(keys, u.from)
	}
	objects, _, err := listAll(ctx, client, u.bucket, u.from+"/", "")
	if err != nil {
		return 0, err
	}
	for _, o := range objects {
		keys = append(keys, o.Key)
	}
	if len(keys) == 0 {
		return 0, fmt.Errorf("no such file or folder")
	}

	dest := make([]string, len(keys))
	for i, k := range keys {
		dest[i] = u.to + k[len(u.from):]
		if !overwrite {
			info, err := statRemote(ctx, client, u.destBucket, dest[i])
			if err != nil {
				return 0, err
			}
			if info.file != nil {
				return 0, fmt.Errorf("%s already exists: use --overwrite to replace it", remoteName(u.destBucket, dest[i]))
			}
		}
	}
	for i, k := range keys {
		if _, err := client.CopyFileObject(ctx, apiclient.FileCopyRequest{
			SourceBucket: u.bucket, SourceKey: k, DestBucket: u.destBucket, DestKey: dest[i],
		}); err != nil {
			return 0, apiError(err)
		}
	}
	for _, k := range keys {
		if err := client.DeleteFileObject(ctx, u.bucket, k); err != nil {
			return 0, fmt.Errorf("copied, but could not remove %s: %w", remoteName(u.bucket, k), apiError(err))
		}
	}
	return len(keys), nil
}
