package command_files

import (
	"context"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

var copyCmd = &cli.Command{
	Name:    "copy",
	Aliases: []string{"cp"},
	Usage:   "Copy files between your machine and buckets, or between buckets",
	Description: `Copy files, giving a bucket's files as bucket:path. The direction follows from which side is a bucket path.

Upload: knot file copy abc.py b1:abc.py

Download: knot file copy b1:dir/abc.py ./test.py

Between buckets, on the server and moving no data: knot file copy b1:dir/abc.py b2:dir2/

Name several sources before the destination, which is then a folder: a local directory, or a bucket path ending in / (or just bucket:, the whole bucket). -r copies directories and bucket folders, and their contents go into the destination folder.

A bucket path with a wildcard (*, ?, [...], and ** for any number of folders) must be quoted: 'b1:logs/*.log'. A wildcard that matches a folder takes everything below it with -r.

Use - as the source to copy stdin, or as the destination to write a single file to stdout. Files already at the destination are replaced. A local path with a colon in its name is written with a leading ./ so it is not read as a bucket.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Copy folders and directories, with everything below them."},
	},
	MinArgs: 2,
	MaxArgs: cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return runCopy(ctx, client, cmd.GetArgs(), cmd.GetBool("recursive"))
	},
}

func contentType(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// runCopy copies the files named by args, the last being the destination.
func runCopy(ctx context.Context, client *apiclient.ApiClient, args []string, recursive bool) error {
	if len(args) < 2 {
		return fmt.Errorf("copy needs a source and a destination, e.g. knot file copy abc.py bucket1:abc.py")
	}
	dst, err := parseLocation(args[len(args)-1])
	if err != nil {
		return err
	}
	srcLocs := make([]location, len(args)-1)
	for i, a := range args[:len(args)-1] {
		if srcLocs[i], err = parseLocation(a); err != nil {
			return err
		}
	}

	for _, l := range srcLocs {
		if l.stdio && len(srcLocs) > 1 {
			return fmt.Errorf("- can only be the one source")
		}
	}
	switch {
	case dst.stdio && srcLocs[0].stdio:
		return fmt.Errorf("- cannot be both the source and the destination")
	case dst.stdio:
		return copyToStdout(ctx, client, srcLocs, recursive)
	case srcLocs[0].stdio:
		return copyFromStdin(ctx, client, dst, recursive)
	}

	if !dst.remote {
		for _, l := range srcLocs {
			if !l.remote {
				return fmt.Errorf("%s and %s are both local: one side must be a bucket path, written bucket:path", l, dst)
			}
		}
	}

	// Expand every source into the files it names.
	var plan []source
	needFolder := len(srcLocs) > 1
	for _, l := range srcLocs {
		var files []source
		var folder bool
		if l.remote {
			files, folder, err = remoteSources(ctx, client, l, recursive, false)
		} else {
			files, folder, err = localSources(l.raw, recursive)
		}
		if err != nil {
			return err
		}
		plan = append(plan, files...)
		needFolder = needFolder || folder
	}

	// Work out where each file goes.
	isFolder, err := destinationIsFolder(dst, needFolder)
	if err != nil {
		return err
	}
	type target struct {
		src source
		key string // remote destination
		dir string // local destination
	}
	targets := make([]target, 0, len(plan))
	seen := make(map[string]string)
	for _, s := range plan {
		t := target{src: s}
		var id string
		switch {
		case dst.remote && isFolder:
			t.key = dst.key + s.rel
			id = t.key
		case dst.remote:
			t.key = dst.key
			id = t.key
		case isFolder:
			if t.dir, err = within(dst.raw, s.rel); err != nil {
				return err
			}
			id = t.dir
		default:
			t.dir = dst.raw
			id = t.dir
		}
		if prev, dup := seen[id]; dup {
			return fmt.Errorf("%s and %s would both be copied to %s", prev, s, id)
		}
		seen[id] = s.String()
		targets = append(targets, t)
	}

	count := 0
	for _, t := range targets {
		s := t.src
		switch {
		case dst.remote && s.local != "":
			err = uploadFile(ctx, client, s.local, dst.bucket, t.key)
		case dst.remote:
			err = serverCopy(ctx, client, s, dst.bucket, t.key)
		case s.marker:
			// An empty folder in a bucket becomes an empty directory.
			if err = os.MkdirAll(t.dir, 0755); err == nil {
				continue
			}
		default:
			err = downloadFile(ctx, client, s.bucket, s.key, t.dir)
		}
		if err != nil {
			return err
		}
		if !s.marker {
			count++
		}
	}
	if count != 1 {
		fmt.Printf("%d files copied\n", count)
	}
	return nil
}

// destinationIsFolder decides whether the destination is a folder the files
// go into, rather than the name of a single file. Several files, or any
// folder or wildcard source, need a folder: the destination is made when it
// is local and does not exist.
func destinationIsFolder(dst location, needFolder bool) (bool, error) {
	if dst.remote {
		if isFolderKey(dst.key) {
			return true, nil
		}
		if needFolder {
			return false, fmt.Errorf("the destination %s must be a folder to copy more than one file: end it with /", dst)
		}
		return false, nil
	}

	trailing := strings.HasSuffix(dst.raw, "/") || strings.HasSuffix(dst.raw, string(filepath.Separator))
	info, err := os.Stat(dst.raw)
	exists := err == nil
	if exists && info.IsDir() {
		return true, nil
	}
	if trailing || needFolder {
		if exists {
			return false, fmt.Errorf("%s exists and is not a directory", dst.raw)
		}
		return true, nil
	}
	return false, nil
}

// uploadFile copies a local file to a bucket.
func uploadFile(ctx context.Context, client *apiclient.ApiClient, local, bucket, key string) error {
	f, err := os.Open(local)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}
	if _, err := client.PutFileObject(ctx, bucket, key, f, info.Size(), contentType(local), info.ModTime()); err != nil {
		return fmt.Errorf("%s: %s", local, cmdutil.CleanAPIError(err))
	}
	fmt.Printf("%s -> %s (%s)\n", local, remoteName(bucket, key), formatBytes(info.Size()))
	return nil
}

// serverCopy copies a file already in a bucket, on the server.
func serverCopy(ctx context.Context, client *apiclient.ApiClient, s source, bucket, key string) error {
	info, err := client.CopyFileObject(ctx, apiclient.FileCopyRequest{SourceBucket: s.bucket, SourceKey: s.key, DestBucket: bucket, DestKey: key})
	if err != nil {
		return fmt.Errorf("%s: %s", s, cmdutil.CleanAPIError(err))
	}
	fmt.Printf("%s -> %s (%s)\n", s, remoteName(bucket, key), formatBytes(info.Size))
	return nil
}

// downloadFile writes a bucket's file to local via a temporary file, or to
// stdout for -.
func downloadFile(ctx context.Context, client *apiclient.ApiClient, bucket, key, local string) error {
	n, err := fetchFile(ctx, client, bucket, key, local)
	if err == nil && local != "-" {
		fmt.Printf("%s -> %s (%s)\n", remoteName(bucket, key), local, formatBytes(n))
	}
	return err
}

// fetchFile is downloadFile without the report: it returns the bytes written.
func fetchFile(ctx context.Context, client *apiclient.ApiClient, bucket, key, local string) (int64, error) {
	resp, err := client.GetFileObject(ctx, bucket, key)
	if err != nil {
		return 0, fmt.Errorf("%s: %s", remoteName(bucket, key), cmdutil.CleanAPIError(err))
	}
	defer resp.Body.Close()

	if local == "-" {
		return io.Copy(os.Stdout, resp.Body)
	}

	if dir := filepath.Dir(local); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return 0, err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(local), ".knot-download-*")
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(tmp, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return 0, err
	}
	// The temporary file is private (0600); give the download the mode of
	// the file it replaces, or the usual 0644.
	mode := os.FileMode(0644)
	if info, err := os.Stat(local); err == nil {
		mode = info.Mode().Perm()
	}
	os.Chmod(tmp.Name(), mode)
	if err := os.Rename(tmp.Name(), local); err != nil {
		os.Remove(tmp.Name())
		return 0, err
	}
	if mtime, ok := apiclient.ParseMtime(resp.Header.Get(apiclient.FileMtimeHeader)); ok {
		os.Chtimes(local, mtime, mtime)
	}
	return n, nil
}

// copyFromStdin uploads stdin to a single bucket file.
func copyFromStdin(ctx context.Context, client *apiclient.ApiClient, dst location, recursive bool) error {
	if recursive {
		return fmt.Errorf("-r cannot be used with stdin (-)")
	}
	if !dst.remote {
		return fmt.Errorf("stdin (-) can only be copied to a bucket path, written bucket:path")
	}
	if isFolderKey(dst.key) || hasGlob(dst.key) {
		return fmt.Errorf("a file name is required when copying stdin: %s", dst)
	}
	if _, err := client.PutFileObject(ctx, dst.bucket, dst.key, os.Stdin, -1, contentType(dst.key), time.Now()); err != nil {
		return apiError(err)
	}
	fmt.Printf("stdin -> %s\n", dst)
	return nil
}

// copyToStdout writes one bucket file to stdout.
func copyToStdout(ctx context.Context, client *apiclient.ApiClient, srcs []location, recursive bool) error {
	if recursive {
		return fmt.Errorf("-r cannot be used with stdout (-); give a directory")
	}
	if len(srcs) != 1 || !srcs[0].remote {
		return fmt.Errorf("stdout (-) takes a single bucket file, written bucket:path")
	}
	files, _, err := remoteSources(ctx, client, srcs[0], false, false)
	if err != nil {
		return err
	}
	if len(files) != 1 || files[0].marker {
		return fmt.Errorf("stdout (-) takes a single file, but %s names %d", srcs[0], len(files))
	}
	return downloadFile(ctx, client, files[0].bucket, files[0].key, "-")
}
