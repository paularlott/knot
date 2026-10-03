package command_files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var syncFlags = []cli.Flag{
	&cli.BoolFlag{Name: "delete", Usage: "Delete files at the destination that are not in the source."},
	&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "Show what would change without changing anything."},
}

var syncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Make a bucket prefix and a local directory match",
	Description: `Copy only what differs between a local directory and a bucket prefix.

Files are compared by size and SHA-256 checksum, so unchanged files are never
transferred; modification times are kept. With --delete, files at the
destination that are not in the source are removed.`,
	Commands: []*cli.Command{syncUpCmd, syncDownCmd},
}

var syncUpCmd = &cli.Command{
	Name:  "up",
	Usage: "Upload a local directory to bucket/prefix",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "directory", Usage: "Local directory", Required: true},
		&cli.StringArg{Name: "destination", Usage: "bucket or bucket/prefix", Required: true},
	},
	Flags:   syncFlags,
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		bucket, prefix, err := syncRemote(cmd.GetStringArg("destination"))
		if err != nil {
			return err
		}
		return syncUp(ctx, client, cmd.GetStringArg("directory"), bucket, prefix, cmd.GetBool("delete"), cmd.GetBool("dry-run"))
	},
}

var syncDownCmd = &cli.Command{
	Name:  "down",
	Usage: "Download bucket/prefix to a local directory",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "source", Usage: "bucket or bucket/prefix", Required: true},
		&cli.StringArg{Name: "directory", Usage: "Local directory, created if missing", Required: true},
	},
	Flags:   syncFlags,
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		bucket, prefix, err := syncRemote(cmd.GetStringArg("source"))
		if err != nil {
			return err
		}
		return syncDown(ctx, client, bucket, prefix, cmd.GetStringArg("directory"), cmd.GetBool("delete"), cmd.GetBool("dry-run"))
	},
}

// syncRemote parses bucket[/prefix], normalising the prefix to end in "/".
func syncRemote(path string) (string, string, error) {
	bucket, prefix, err := parseRemote(path)
	return bucket, dirPrefix(prefix), err
}

// fileSHA256 returns the hex SHA-256 of a local file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// sameContent reports whether a local file holds the given remote content.
func sameContent(local string, size int64, sha string) bool {
	info, err := os.Stat(local)
	if err != nil || !info.Mode().IsRegular() || info.Size() != size {
		return false
	}
	got, err := fileSHA256(local)
	return err == nil && got == sha
}

type syncCounts struct {
	copied, unchanged, deleted int
}

func (c syncCounts) report(verb string, dryRun bool) {
	prefix := ""
	if dryRun {
		prefix = "dry run: would have "
	}
	fmt.Printf("%s%s %d, unchanged %d, deleted %d\n", prefix, verb, c.copied, c.unchanged, c.deleted)
}

func remoteIndex(ctx context.Context, client *apiclient.ApiClient, bucket, prefix string) (map[string]apiclient.FileObjectInfo, error) {
	objects, _, err := listAll(ctx, client, bucket, prefix, "")
	if err != nil {
		return nil, err
	}
	index := make(map[string]apiclient.FileObjectInfo, len(objects))
	for _, o := range objects {
		index[strings.TrimPrefix(o.Key, prefix)] = o
	}
	return index, nil
}

func syncUp(ctx context.Context, client *apiclient.ApiClient, dir, bucket, prefix string, del, dryRun bool) error {
	// Walk the real directory: a walk does not descend into a symlinked
	// root, which would look empty and, with --delete, empty the prefix.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}

	remote, err := remoteIndex(ctx, client, bucket, prefix)
	if err != nil {
		return err
	}

	var counts syncCounts
	seen := make(map[string]bool)
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		seen[rel] = true

		if o, ok := remote[rel]; ok && sameContent(p, o.Size, o.SHA256) {
			counts.unchanged++
			return nil
		}
		counts.copied++
		if dryRun {
			fmt.Printf("upload %s -> %s/%s\n", p, bucket, prefix+rel)
			return nil
		}
		return uploadFile(ctx, client, p, bucket, prefix+rel)
	})
	if err != nil {
		return err
	}

	if del {
		for rel := range remote {
			if seen[rel] {
				continue
			}
			counts.deleted++
			if dryRun {
				fmt.Printf("delete %s/%s\n", bucket, prefix+rel)
				continue
			}
			if err := client.DeleteFileObject(ctx, bucket, prefix+rel); err != nil {
				return fmt.Errorf("%s/%s: %s", bucket, prefix+rel, cmdutil.CleanAPIError(err))
			}
			fmt.Printf("deleted %s/%s\n", bucket, prefix+rel)
		}
	}

	counts.report("uploaded", dryRun)
	return nil
}

func syncDown(ctx context.Context, client *apiclient.ApiClient, bucket, prefix, dir string, del, dryRun bool) error {
	remote, err := remoteIndex(ctx, client, bucket, prefix)
	if err != nil {
		return err
	}

	var counts syncCounts
	for rel, o := range remote {
		local, err := within(dir, rel)
		if err != nil {
			return err
		}

		if sameContent(local, o.Size, o.SHA256) {
			counts.unchanged++
			// Keep the modification time in step with the bucket.
			if !dryRun && !o.ModifiedAt.IsZero() {
				if info, err := os.Stat(local); err == nil && info.ModTime().Sub(o.ModifiedAt).Abs() > time.Second {
					os.Chtimes(local, o.ModifiedAt, o.ModifiedAt)
				}
			}
			continue
		}
		counts.copied++
		if dryRun {
			fmt.Printf("download %s/%s -> %s\n", bucket, o.Key, local)
			continue
		}
		if err := downloadFile(ctx, client, bucket, o.Key, local); err != nil {
			return err
		}
	}

	if del {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			dir = real
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() || !d.Type().IsRegular() {
					return err
				}
				rel, err := filepath.Rel(dir, p)
				if err != nil {
					return err
				}
				if _, ok := remote[filepath.ToSlash(rel)]; ok {
					return nil
				}
				counts.deleted++
				if dryRun {
					fmt.Printf("delete %s\n", p)
					return nil
				}
				if err := os.Remove(p); err != nil {
					return err
				}
				fmt.Printf("deleted %s\n", p)
				return nil
			})
			if err != nil {
				return err
			}
		}
	}

	counts.report("downloaded", dryRun)
	return nil
}
