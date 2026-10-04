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
	Usage: "Make a bucket folder and a local directory, or two bucket folders, match",
	Description: `Copy only what differs from the source to the destination. Either side is a local directory or a bucket folder, bucket:folder (bucket: is a whole bucket), and the direction follows from which is which.

Upload: knot file sync ./site b1:site

Download: knot file sync b1:site ./site

Between buckets, on the server: knot file sync b1:site b2:backup

Files are compared by size and SHA-256 checksum, so unchanged files are never transferred; modification times are kept. With --delete, files at the destination that are not in the source are removed.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "source", Usage: "Local directory or bucket:folder", Required: true},
		&cli.StringArg{Name: "destination", Usage: "Local directory or bucket:folder", Required: true},
	},
	Flags:   syncFlags,
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return runSync(ctx, client, cmd.GetStringArg("source"), cmd.GetStringArg("destination"), cmd.GetBool("delete"), cmd.GetBool("dry-run"))
	},
}

// runSync makes the destination match the source, in the direction the two
// paths imply.
func runSync(ctx context.Context, client *apiclient.ApiClient, srcArg, dstArg string, del, dryRun bool) error {
	src, err := parseLocation(srcArg)
	if err != nil {
		return err
	}
	dst, err := parseLocation(dstArg)
	if err != nil {
		return err
	}
	for _, l := range []location{src, dst} {
		if l.stdio {
			return fmt.Errorf("sync works on directories and folders, not stdin or stdout")
		}
		if l.remote && hasGlob(l.key) {
			return fmt.Errorf("sync takes a folder, not a pattern: %s", l)
		}
	}

	switch {
	case !src.remote && dst.remote:
		return syncUp(ctx, client, src.raw, dst.bucket, dirPrefix(dst.key), del, dryRun)
	case src.remote && !dst.remote:
		return syncDown(ctx, client, src.bucket, dirPrefix(src.key), dst.raw, del, dryRun)
	case src.remote && dst.remote:
		return syncBuckets(ctx, client, src.bucket, dirPrefix(src.key), dst.bucket, dirPrefix(dst.key), del, dryRun)
	}
	return fmt.Errorf("%s and %s are both local: one side must be a bucket path, written bucket:folder", src, dst)
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
			fmt.Printf("upload %s -> %s\n", p, remoteName(bucket, prefix+rel))
			return nil
		}
		return uploadFile(ctx, client, p, bucket, prefix+rel)
	})
	if err != nil {
		return err
	}

	if del {
		for rel := range remote {
			// Empty folder markers are not files; leave them be.
			if seen[rel] || rel == "" || strings.HasSuffix(rel, "/") {
				continue
			}
			counts.deleted++
			if dryRun {
				fmt.Printf("delete %s\n", remoteName(bucket, prefix+rel))
				continue
			}
			if err := client.DeleteFileObject(ctx, bucket, prefix+rel); err != nil {
				return fmt.Errorf("%s: %s", remoteName(bucket, prefix+rel), cmdutil.CleanAPIError(err))
			}
			fmt.Printf("deleted %s\n", remoteName(bucket, prefix+rel))
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

		// An empty folder in the bucket is an empty directory here.
		if rel == "" || strings.HasSuffix(rel, "/") {
			if rel != "" && !dryRun {
				if err := os.MkdirAll(local, 0755); err != nil {
					return err
				}
			}
			continue
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
			fmt.Printf("download %s -> %s\n", remoteName(bucket, o.Key), local)
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

// syncBuckets makes one bucket folder match another on the server, copying
// only the files whose content differs, so no data is transferred.
func syncBuckets(ctx context.Context, client *apiclient.ApiClient, srcBucket, srcPrefix, dstBucket, dstPrefix string, del, dryRun bool) error {
	// The two may name one bucket in short and full form, so compare the
	// full names, and refuse a sync of a folder into itself or its own contents.
	sb, err := client.GetFileBucket(ctx, srcBucket)
	if err != nil {
		return apiError(err)
	}
	db, err := client.GetFileBucket(ctx, dstBucket)
	if err != nil {
		return apiError(err)
	}
	if sb.Name == db.Name && (strings.HasPrefix(srcPrefix, dstPrefix) || strings.HasPrefix(dstPrefix, srcPrefix)) {
		return fmt.Errorf("%s and %s overlap", remoteName(srcBucket, srcPrefix), remoteName(dstBucket, dstPrefix))
	}

	src, err := remoteIndex(ctx, client, srcBucket, srcPrefix)
	if err != nil {
		return err
	}
	dst, err := remoteIndex(ctx, client, dstBucket, dstPrefix)
	if err != nil {
		return err
	}

	var counts syncCounts
	for rel, o := range src {
		if rel == "" {
			continue // the folder's own marker
		}
		if d, ok := dst[rel]; ok && (d.SHA256 == o.SHA256 && d.Size == o.Size) {
			counts.unchanged++
			continue
		}
		counts.copied++
		if dryRun {
			fmt.Printf("copy %s -> %s\n", remoteName(srcBucket, o.Key), remoteName(dstBucket, dstPrefix+rel))
			continue
		}
		if err := serverCopy(ctx, client, source{bucket: srcBucket, key: o.Key}, dstBucket, dstPrefix+rel); err != nil {
			return err
		}
	}

	if del {
		for rel := range dst {
			if _, ok := src[rel]; ok || rel == "" {
				continue
			}
			counts.deleted++
			if dryRun {
				fmt.Printf("delete %s\n", remoteName(dstBucket, dstPrefix+rel))
				continue
			}
			if err := client.DeleteFileObject(ctx, dstBucket, dstPrefix+rel); err != nil {
				return fmt.Errorf("%s: %s", remoteName(dstBucket, dstPrefix+rel), cmdutil.CleanAPIError(err))
			}
			fmt.Printf("deleted %s\n", remoteName(dstBucket, dstPrefix+rel))
		}
	}

	counts.report("copied", dryRun)
	return nil
}
