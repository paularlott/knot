package command_files

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util"
)

// listing is what ls shows for a bucket path.
type listing struct {
	Bucket   string                     `json:"bucket"`
	Prefix   string                     `json:"prefix"`
	Prefixes []string                   `json:"prefixes"`
	Objects  []apiclient.FileObjectInfo `json:"objects"`
}

var lsCmd = &cli.Command{
	Name:        "ls",
	Aliases:     []string{"list"},
	Usage:       "List buckets or the files in a bucket",
	Description: `With no argument lists your buckets. With bucket:path lists the files below it, one level unless -r is given; bucket: is the whole bucket, and a path may hold wildcards, quoted: 'bucket:logs/*.log'.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "path", Usage: "bucket:path", Required: false},
	},
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "List every file below the path instead of one level."},
		&cli.BoolFlag{Name: "all", Usage: "With no path, list every bucket (file administrators only)."},
		jsonFlag,
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}

		arg := cmd.GetStringArg("path")
		if arg == "" {
			return listBuckets(ctx, client, cmd.GetBool("all"), cmd.GetBool("json"))
		}
		loc, err := requireRemote(arg)
		if err != nil {
			return err
		}
		l, err := listRemote(ctx, client, loc, cmd.GetBool("recursive"))
		if err != nil {
			return err
		}

		if cmd.GetBool("json") {
			return printJSON(l)
		}
		table := [][]string{{"SIZE", "MODIFIED", "NAME"}}
		for _, p := range l.Prefixes {
			table = append(table, []string{"DIR", "", p})
		}
		for _, o := range l.Objects {
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

// listRemote lists what a bucket path names: the contents of a folder, the
// files a wildcard matches, or a single file.
func listRemote(ctx context.Context, client *apiclient.ApiClient, loc location, recursive bool) (*listing, error) {
	l := &listing{Bucket: loc.bucket, Prefix: loc.key, Prefixes: []string{}, Objects: []apiclient.FileObjectInfo{}}
	delimiter := "/"
	if recursive {
		delimiter = ""
	}

	if hasGlob(loc.key) {
		root, _ := globRoot(loc.key)
		objects, _, err := listAll(ctx, client, loc.bucket, root, "")
		if err != nil {
			return nil, err
		}
		seen := make(map[string]bool)
		for _, o := range objects {
			if _, ok := matchGlob(loc.key, o.Key, recursive); ok {
				l.Objects = append(l.Objects, o)
			} else if f, ok := globFolder(loc.key, o.Key); ok && !seen[f] {
				seen[f] = true
				l.Prefixes = append(l.Prefixes, f)
			}
		}
		if len(l.Objects) == 0 && len(l.Prefixes) == 0 {
			return nil, fmt.Errorf("no files match %s", loc)
		}
		return l, nil
	}

	prefix := loc.key
	if !isFolderKey(loc.key) {
		info, err := statRemote(ctx, client, loc.bucket, loc.key)
		if err != nil {
			return nil, err
		}
		switch {
		case info.folder:
			prefix = dirPrefix(loc.key)
			l.Prefix = prefix
		case info.file != nil:
			l.Objects = append(l.Objects, *info.file)
			return l, nil
		default:
			return nil, fmt.Errorf("%s: no such file or folder", loc)
		}
	}

	objects, prefixes, err := listAll(ctx, client, loc.bucket, prefix, delimiter)
	if err != nil {
		return nil, err
	}
	for _, o := range objects {
		if o.Key != prefix { // not the folder's own marker
			l.Objects = append(l.Objects, o)
		}
	}
	l.Prefixes = append(l.Prefixes, prefixes...)
	return l, nil
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

var catCmd = &cli.Command{
	Name:        "cat",
	Usage:       "Write files to stdout",
	Description: "Write bucket files to stdout, one after another: knot file cat bucket:path/file.txt",
	MinArgs:     1,
	MaxArgs:     cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		var files []source
		for _, arg := range cmd.GetArgs() {
			loc, err := requireRemote(arg)
			if err != nil {
				return err
			}
			list, _, err := remoteSources(ctx, client, loc, false, false)
			if err != nil {
				return err
			}
			files = append(files, list...)
		}
		for _, f := range files {
			if f.marker {
				continue
			}
			if err := downloadFile(ctx, client, f.bucket, f.key, "-"); err != nil {
				return err
			}
		}
		return nil
	},
}

var rmCmd = &cli.Command{
	Name:        "rm",
	Aliases:     []string{"delete"},
	Usage:       "Delete files",
	Description: `Delete bucket files: knot file rm bucket:path/file.txt. A path may hold wildcards, quoted: 'bucket:logs/*.tmp'. -r deletes a folder and everything below it; bucket: with -r empties the bucket (it stays: see knot file bucket delete).`,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Delete folders, with everything below them."},
	},
	MinArgs: 1,
	MaxArgs: cli.UnlimitedArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		recursive := cmd.GetBool("recursive")

		// Find everything first, so a path that matches nothing deletes nothing.
		var files []source
		for _, arg := range cmd.GetArgs() {
			loc, err := requireRemote(arg)
			if err != nil {
				return err
			}
			list, _, err := remoteSources(ctx, client, loc, recursive, true)
			if err != nil {
				return err
			}
			files = append(files, list...)
		}

		count := 0
		for _, f := range files {
			if err := client.DeleteFileObject(ctx, f.bucket, f.key); err != nil {
				return fmt.Errorf("%s: %s", f, cmdutil.CleanAPIError(err))
			}
			fmt.Printf("deleted %s\n", f)
			if !strings.HasSuffix(f.key, "/") {
				count++
			}
		}
		if len(files) > 1 {
			fmt.Printf("%d files deleted\n", count)
		}
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
