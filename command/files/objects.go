package command_files

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

var lsCmd = &cli.Command{
	Name:        "ls",
	Usage:       "List buckets or the files in a bucket",
	Description: "With no argument lists your buckets; with bucket[/prefix] lists the files below it.",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "path", Usage: "bucket or bucket/prefix", Required: false},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "List every file below the prefix instead of one level."},
		&cli.BoolFlag{Name: "all", Usage: "With no path, list every bucket (file administrators only)."},
		jsonFlag,
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}

		remote := cmd.GetStringArg("path")
		if remote == "" {
			return listBuckets(ctx, client, cmd.GetBool("all"), cmd.GetBool("json"))
		}

		bucket, prefix, err := parseRemote(remote)
		if err != nil {
			return err
		}
		delimiter := "/"
		if cmd.GetBool("recursive") {
			delimiter = ""
		}

		objects, prefixes, err := listAll(ctx, client, bucket, prefix, delimiter)
		if err != nil {
			return err
		}
		if cmd.GetBool("json") {
			if objects == nil {
				objects = []apiclient.FileObjectInfo{}
			}
			if prefixes == nil {
				prefixes = []string{}
			}
			return printJSON(struct {
				Bucket   string                     `json:"bucket"`
				Prefix   string                     `json:"prefix"`
				Prefixes []string                   `json:"prefixes"`
				Objects  []apiclient.FileObjectInfo `json:"objects"`
			}{bucket, prefix, prefixes, objects})
		}
		table := [][]string{{"SIZE", "MODIFIED", "NAME"}}
		for _, p := range prefixes {
			table = append(table, []string{"DIR", "", p})
		}
		for _, o := range objects {
			table = append(table, []string{formatBytes(o.Size), o.ModifiedAt.Local().Format("2006-01-02 15:04:05"), o.Key})
		}

		if len(table) == 1 {
			fmt.Println("No files found")
			return nil
		}
		util.PrintTable(table)
		return nil
	},
}

func contentType(name string) string {
	if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
		return t
	}
	return "application/octet-stream"
}

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
	fmt.Printf("%s -> %s/%s (%s)\n", local, bucket, key, formatBytes(info.Size()))
	return nil
}

var putCmd = &cli.Command{
	Name:  "put",
	Usage: "Upload files",
	Description: `Upload a file or, with -r, a directory to bucket/key.

If the destination is a bucket or ends with / the file keeps its name. Use - as
the source to upload stdin.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "source", Usage: "Local file, directory or - for stdin", Required: true},
		&cli.StringArg{Name: "destination", Usage: "bucket or bucket/key", Required: true},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Upload a directory and everything below it."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}

		source := cmd.GetStringArg("source")
		bucket, key, err := parseRemote(cmd.GetStringArg("destination"))
		if err != nil {
			return err
		}

		if source == "-" {
			if cmd.GetBool("recursive") {
				return fmt.Errorf("-r cannot be used with stdin (-)")
			}
			if key == "" || strings.HasSuffix(key, "/") {
				return fmt.Errorf("a key is required when uploading stdin")
			}
			if _, err := client.PutFileObject(ctx, bucket, key, os.Stdin, -1, contentType(key), time.Now()); err != nil {
				return apiError(err)
			}
			fmt.Printf("stdin -> %s/%s\n", bucket, key)
			return nil
		}

		info, err := os.Stat(source)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			if key == "" || strings.HasSuffix(key, "/") {
				key += filepath.Base(source)
			}
			return uploadFile(ctx, client, source, bucket, key)
		}

		if !cmd.GetBool("recursive") {
			return fmt.Errorf("%s is a directory, use -r to upload it", source)
		}
		key = dirPrefix(key)
		count := 0
		// Walk the real directory; a symlinked root would look empty.
		if source, err = filepath.EvalSymlinks(source); err != nil {
			return err
		}
		err = filepath.WalkDir(source, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, err := filepath.Rel(source, p)
			if err != nil {
				return err
			}
			count++
			return uploadFile(ctx, client, p, bucket, key+filepath.ToSlash(rel))
		})
		if err != nil {
			return err
		}
		fmt.Printf("%d files uploaded\n", count)
		return nil
	},
}

// downloadFile writes an object to local via a temporary file.
func downloadFile(ctx context.Context, client *apiclient.ApiClient, bucket, key, local string) error {
	resp, err := client.GetFileObject(ctx, bucket, key)
	if err != nil {
		return fmt.Errorf("%s/%s: %s", bucket, key, cmdutil.CleanAPIError(err))
	}
	defer resp.Body.Close()

	if local == "-" {
		_, err := io.Copy(os.Stdout, resp.Body)
		return err
	}

	if dir := filepath.Dir(local); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(local), ".knot-download-*")
	if err != nil {
		return err
	}
	n, err := io.Copy(tmp, resp.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
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
		return err
	}
	if mtime, ok := apiclient.ParseMtime(resp.Header.Get(apiclient.FileMtimeHeader)); ok {
		os.Chtimes(local, mtime, mtime)
	}
	fmt.Printf("%s/%s -> %s (%s)\n", bucket, key, local, formatBytes(n))
	return nil
}

// listAll lists everything below prefix, every page of it; with a
// delimiter, deeper keys are rolled up into prefixes.
func listAll(ctx context.Context, client *apiclient.ApiClient, bucket, prefix, delimiter string) ([]apiclient.FileObjectInfo, []string, error) {
	var objects []apiclient.FileObjectInfo
	var prefixes []string
	for after := ""; ; {
		list, err := client.ListFileObjects(ctx, bucket, prefix, delimiter, after, 1000)
		if err != nil {
			return nil, nil, apiError(err)
		}
		objects = append(objects, list.Objects...)
		prefixes = append(prefixes, list.Prefixes...)
		if !list.IsTruncated {
			return objects, prefixes, nil
		}
		if list.Next == "" || list.Next <= after {
			return nil, nil, fmt.Errorf("listing %s did not advance", bucket)
		}
		after = list.Next
	}
}

var getCmd = &cli.Command{
	Name:  "get",
	Usage: "Download files",
	Description: `Download bucket/key to a local file or, with -r, everything below a prefix
into a local directory. The destination defaults to the current directory; use
- to write a single file to stdout.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "source", Usage: "bucket/key, or bucket[/prefix] with -r", Required: true},
		&cli.StringArg{Name: "destination", Usage: "Local file or directory", Required: false},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Download everything below the prefix."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}

		bucket, key, err := parseRemote(cmd.GetStringArg("source"))
		if err != nil {
			return err
		}
		dest := cmd.GetStringArg("destination")

		if cmd.GetBool("recursive") {
			if dest == "-" {
				return fmt.Errorf("-r cannot be used with stdout (-); give a directory")
			}
			if dest == "" {
				dest = "."
			}
			prefix := dirPrefix(key)
			objects, _, err := listAll(ctx, client, bucket, prefix, "")
			if err != nil {
				return err
			}
			for _, o := range objects {
				local, err := within(dest, strings.TrimPrefix(o.Key, prefix))
				if err != nil {
					return err
				}
				if err := downloadFile(ctx, client, bucket, o.Key, local); err != nil {
					return err
				}
			}
			fmt.Printf("%d files downloaded\n", len(objects))
			return nil
		}

		if key == "" || strings.HasSuffix(key, "/") {
			return fmt.Errorf("a key is required, use -r to download a prefix")
		}
		if dest == "" {
			dest = path.Base(key)
		} else if info, err := os.Stat(dest); err == nil && info.IsDir() {
			dest = filepath.Join(dest, path.Base(key))
		}
		return downloadFile(ctx, client, bucket, key, dest)
	},
}

var catCmd = &cli.Command{
	Name:  "cat",
	Usage: "Write a file to stdout",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "path", Usage: "bucket/key", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		bucket, key, err := parseRemote(cmd.GetStringArg("path"))
		if err != nil {
			return err
		}
		if key == "" || strings.HasSuffix(key, "/") {
			return fmt.Errorf("a key is required, not a folder")
		}
		return downloadFile(ctx, client, bucket, key, "-")
	},
}

var rmCmd = &cli.Command{
	Name:  "rm",
	Usage: "Delete files",
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "path", Usage: "bucket/key, or bucket[/prefix] with -r", Required: true},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Delete everything below the prefix."},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		bucket, key, err := parseRemote(cmd.GetStringArg("path"))
		if err != nil {
			return err
		}

		if !cmd.GetBool("recursive") {
			if key == "" || strings.HasSuffix(key, "/") {
				return fmt.Errorf("a key is required, use -r to delete a prefix")
			}
			if err := client.DeleteFileObject(ctx, bucket, key); err != nil {
				return apiError(err)
			}
			fmt.Printf("deleted %s/%s\n", bucket, key)
			return nil
		}

		prefix := dirPrefix(key)
		objects, _, err := listAll(ctx, client, bucket, prefix, "")
		if err != nil {
			return err
		}
		for _, o := range objects {
			if err := client.DeleteFileObject(ctx, bucket, o.Key); err != nil {
				return fmt.Errorf("%s/%s: %s", bucket, o.Key, cmdutil.CleanAPIError(err))
			}
			fmt.Printf("deleted %s/%s\n", bucket, o.Key)
		}
		fmt.Printf("%d files deleted\n", len(objects))
		return nil
	},
}

var usageCmd = &cli.Command{
	Name:    "usage",
	Usage:   "Show your file storage usage and quota",
	Flags:   []cli.Flag{jsonFlag},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		usage, err := client.GetFileUsage(ctx)
		if err != nil {
			return apiError(err)
		}
		if cmd.GetBool("json") {
			return printJSON(usage)
		}
		quota := "unlimited"
		if usage.QuotaBytes > 0 {
			quota = fmt.Sprintf("%s (%.1f%% used)", formatBytes(usage.QuotaBytes), float64(usage.UsedBytes)*100/float64(usage.QuotaBytes))
		}
		buckets := fmt.Sprint(usage.Buckets)
		if usage.MaxBuckets > 0 {
			buckets += fmt.Sprintf(" / %d", usage.MaxBuckets)
		}
		util.PrintTable([][]string{
			{"BUCKETS", "FILES", "USED", "QUOTA"},
			{buckets, fmt.Sprint(usage.Objects), formatBytes(usage.UsedBytes), quota},
		})
		return nil
	},
}
