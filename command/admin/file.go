package commands_admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/backupfile"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"
)

// FileCmd is knot admin file: working with file storage backups and checking
// the files on a running server.
var FileCmd = &cli.Command{
	Name:        "file",
	Usage:       "List and restore files from a backup, and check file storage",
	Description: "Work with the file storage in a backup made by knot admin backup, and check and repair file storage on a running server.",
	MaxArgs:     cli.NoArgs,
	Commands: []*cli.Command{
		fileLsCmd,
		fileRestoreCmd,
		FsckCmd,
	},
}

// backupBucket and backupObject are the parts of the records the file
// commands use.
type backupBucket struct {
	Id      string `json:"id"`
	Name    string `json:"name"`
	OwnerId string `json:"owner_id"`
}

type backupObject struct {
	BucketId    string    `json:"bucket_id"`
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	ModifiedAt  time.Time `json:"modified_at"`
}

var encryptKeyFlag = &cli.StringFlag{
	Name: "encrypt-key", Aliases: []string{"e"}, Usage: "The key the backup's records were encrypted with.",
	EnvVars: []string{config.CONFIG_ENV_PREFIX + "_RESTORE_ENCRYPT_KEY"},
}

// openBackupFiles reads the buckets of a backup.
func openBackupFiles(dir, key string) ([]backupBucket, error) {
	m, err := backupfile.ReadManifest(dir)
	if err != nil {
		return nil, err
	}
	if m.Encrypted && key == "" {
		return nil, errors.New("the backup is encrypted: give the key with --encrypt-key")
	}
	if _, ok := m.Counts["file-objects"]; !ok {
		return nil, errors.New("the backup holds no file storage")
	}
	r, err := backupfile.Open(dir, "file-buckets", key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	var buckets []backupBucket
	err = r.Each(func(line []byte) error {
		var b backupBucket
		if err := json.Unmarshal(line, &b); err != nil {
			return err
		}
		buckets = append(buckets, b)
		return nil
	})
	return buckets, err
}

// eachBackupObject calls fn with every file record of the backup.
func eachBackupObject(dir, key string, fn func(o *backupObject, raw []byte) error) error {
	r, err := backupfile.Open(dir, "file-objects", key)
	if err != nil {
		return err
	}
	defer r.Close()
	return r.Each(func(line []byte) error {
		o := &backupObject{}
		if err := json.Unmarshal(line, o); err != nil {
			return err
		}
		return fn(o, line)
	})
}

// findBucket looks a bucket up by name or id.
func findBucket(buckets []backupBucket, name string) *backupBucket {
	for i := range buckets {
		if buckets[i].Name == name || buckets[i].Id == name {
			return &buckets[i]
		}
	}
	return nil
}

// splitBucketPath splits bucket:path.
func splitBucketPath(arg string) (bucket, p string, err error) {
	i := strings.Index(arg, ":")
	if i <= 0 {
		return "", "", fmt.Errorf("%q is not bucket:path; the bucket is its full name, e.g. paul--configs:app/settings.toml", arg)
	}
	return arg[:i], arg[i+1:], nil
}

var fileLsCmd = &cli.Command{
	Name:    "ls",
	Aliases: []string{"list"},
	Usage:   "List the buckets or files in a backup",
	Description: `List what a backup folder holds, without a server: with no path the buckets, else the files below bucket:path, one level unless -r is given. Buckets are named in full, as <username>--<name>, as they were when the backup was taken.

Restore what you find with knot admin file restore.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "backupdir", Usage: "The backup folder", Required: true},
		&cli.StringArg{Name: "path", Usage: "bucket:path", Required: false},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "List every file below the path instead of one level."},
		&cli.BoolFlag{Name: "json", Usage: "Print the result as JSON."},
		encryptKeyFlag,
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		dir, key := cmd.GetStringArg("backupdir"), cmd.GetString("encrypt-key")
		buckets, err := openBackupFiles(dir, key)
		if err != nil {
			return err
		}
		arg := cmd.GetStringArg("path")

		if arg == "" {
			type stats struct {
				files int
				size  int64
			}
			counts := map[string]*stats{}
			if err := eachBackupObject(dir, key, func(o *backupObject, _ []byte) error {
				s := counts[o.BucketId]
				if s == nil {
					s = &stats{}
					counts[o.BucketId] = s
				}
				s.files++
				s.size += o.Size
				return nil
			}); err != nil {
				return err
			}
			sort.Slice(buckets, func(i, j int) bool { return buckets[i].Name < buckets[j].Name })
			if cmd.GetBool("json") {
				out := []map[string]any{}
				for _, b := range buckets {
					s := counts[b.Id]
					if s == nil {
						s = &stats{}
					}
					out = append(out, map[string]any{"id": b.Id, "name": b.Name, "owner_id": b.OwnerId, "files": s.files, "size": s.size})
				}
				return printJSON(out)
			}
			table := [][]string{{"BUCKET", "FILES", "SIZE"}}
			for _, b := range buckets {
				s := counts[b.Id]
				if s == nil {
					s = &stats{}
				}
				table = append(table, []string{b.Name, fmt.Sprint(s.files), humanBytes(s.size)})
			}
			util.PrintTable(table)
			return nil
		}

		bucketName, prefix, err := splitBucketPath(arg)
		if err != nil {
			return err
		}
		b := findBucket(buckets, bucketName)
		if b == nil {
			return fmt.Errorf("the backup has no bucket %s", bucketName)
		}

		folders, files, err := listPath(dir, key, b.Id, prefix, cmd.GetBool("recursive"))
		if err != nil {
			return err
		}
		if len(files) == 0 && len(folders) == 0 {
			return fmt.Errorf("no files at %s", arg)
		}

		if cmd.GetBool("json") {
			if folders == nil {
				folders = []string{}
			}
			if files == nil {
				files = []backupObject{}
			}
			return printJSON(map[string]any{"bucket": b.Name, "prefix": prefix, "folders": folders, "files": files})
		}
		table := [][]string{{"SIZE", "MODIFIED", "NAME"}}
		for _, f := range folders {
			table = append(table, []string{"DIR", "", f})
		}
		for _, o := range files {
			table = append(table, []string{humanBytes(o.Size), o.ModifiedAt.Local().Format("2006-01-02 15:04:05"), o.Key})
		}
		util.PrintTable(table)
		return nil
	},
}

var fileRestoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "Restore a file or folder from a backup into the running server",
	Description: `Copy files from a backup folder back into a running server: one file, bucket:path/file.txt, or a folder with -r, bucket:path/. They go back to the bucket they came from, found by its id, so it works after the owner has been renamed; the bucket must still exist. Use --to to restore into another bucket, or another folder, instead, for example to look at them first.

Files already on the server are left alone and reported unless --overwrite is given, so nothing is replaced without your say-so. With --dry-run nothing is changed: it lists what would be restored.

The files are written as the user the command connects as, who needs the Manage File Storage permission to write into other users' buckets. The owner's quota is charged. Choose the server with --server and --token, or --alias for one in the configuration file; with neither, the default alias is used.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "backupdir", Usage: "The backup folder", Required: true},
		&cli.StringArg{Name: "path", Usage: "bucket:path", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "recursive", Aliases: []string{"r"}, Usage: "Restore a folder and everything below it."},
		&cli.BoolFlag{Name: "overwrite", Usage: "Replace files that already exist."},
		&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "Show what would be restored and change nothing."},
		&cli.StringFlag{Name: "to", Usage: "Restore into this bucket:path instead of where the files came from."},
		encryptKeyFlag,
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		dir, key := cmd.GetStringArg("backupdir"), cmd.GetString("encrypt-key")
		buckets, err := openBackupFiles(dir, key)
		if err != nil {
			return err
		}
		bucketName, prefix, err := splitBucketPath(cmd.GetStringArg("path"))
		if err != nil {
			return err
		}
		src := findBucket(buckets, bucketName)
		if src == nil {
			return fmt.Errorf("the backup has no bucket %s", bucketName)
		}
		if prefix == "" {
			return errors.New("name a file or folder: to restore a whole bucket use bucket:/ with -r")
		}

		recursive := cmd.GetBool("recursive")
		folder := strings.HasSuffix(prefix, "/")
		if prefix == "/" {
			prefix = ""
			folder = true
		}

		// What the path names: a file, or a folder.
		var matches []backupObject
		exact := false
		if err := eachBackupObject(dir, key, func(o *backupObject, _ []byte) error {
			if o.BucketId != src.Id {
				return nil
			}
			switch {
			case o.Key == prefix && !folder:
				exact = true
				matches = append(matches, *o)
			case folder && strings.HasPrefix(o.Key, prefix):
				matches = append(matches, *o)
			case !folder && strings.HasPrefix(o.Key, prefix+"/"):
				matches = append(matches, *o)
			}
			return nil
		}); err != nil {
			return err
		}
		if len(matches) == 0 {
			return fmt.Errorf("no files at %s in the backup", cmd.GetStringArg("path"))
		}
		isFolder := folder || !exact
		if isFolder && !recursive {
			return fmt.Errorf("%s is a folder: use -r to restore it with everything below it", cmd.GetStringArg("path"))
		}
		srcPrefix := prefix
		if isFolder && !strings.HasSuffix(srcPrefix, "/") && srcPrefix != "" {
			srcPrefix += "/"
		}

		// Where they go.
		client, err := adminClient(cmd)
		if err != nil {
			return err
		}
		live, err := client.GetFileBuckets(ctx, true)
		if err != nil {
			return fmt.Errorf("couldn't list the server's buckets: %w", cmdutil.CleanErr(err))
		}
		destName, destPrefix := "", ""
		if to := cmd.GetString("to"); to != "" {
			b, p, err := splitBucketPath(to)
			if err != nil {
				if strings.Contains(to, ":") {
					return err
				}
				b, p = to, ""
			}
			destName, destPrefix = b, p
		}
		var dest *apiclient.FileBucketInfo
		for i := range live.Buckets {
			lb := &live.Buckets[i]
			switch {
			case destName != "" && (lb.Name == destName || lb.Id == destName):
				dest = lb
			case destName == "" && lb.Id == src.Id:
				dest = lb
			case destName == "" && dest == nil && lb.Name == src.Name:
				dest = lb // a bucket of the same name, if the id is not found
			}
		}
		if dest == nil {
			if destName != "" {
				return fmt.Errorf("the server has no bucket %s", destName)
			}
			return fmt.Errorf("the bucket %s no longer exists on the server: create it, or restore into another with --to", src.Name)
		}

		keyFor := func(k string) string {
			if destPrefix == "" && cmd.GetString("to") == "" {
				return k
			}
			if !isFolder {
				if destPrefix == "" || strings.HasSuffix(destPrefix, "/") {
					return destPrefix + path.Base(k)
				}
				return destPrefix
			}
			dp := destPrefix
			if dp != "" && !strings.HasSuffix(dp, "/") {
				dp += "/"
			}
			return dp + strings.TrimPrefix(k, srcPrefix)
		}

		dry, overwrite := cmd.GetBool("dry-run"), cmd.GetBool("overwrite")
		var mu sync.Mutex
		restored, exists, noContent, failed := 0, 0, 0, 0
		var problems []string
		note := func(format string, args ...any) {
			problems = append(problems, fmt.Sprintf(format, args...))
		}

		jobs := make(chan backupObject)
		var wg sync.WaitGroup
		for i := 0; i < 4; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for o := range jobs {
					dst := keyFor(o.Key)
					if dry {
						mu.Lock()
						fmt.Printf("would restore %s -> %s:%s (%s)\n", o.Key, dest.Name, dst, humanBytes(o.Size))
						restored++
						mu.Unlock()
						continue
					}
					outcome, err := restoreFile(ctx, client, dir, dest.Name, dst, &o, overwrite)
					mu.Lock()
					switch outcome {
					case "restored":
						restored++
						fmt.Printf("%s -> %s:%s (%s)\n", o.Key, dest.Name, dst, humanBytes(o.Size))
					case "exists":
						exists++
						fmt.Printf("%s:%s exists, skipped\n", dest.Name, dst)
					case "no-content":
						noContent++
						note("%s: its content is not in the backup", o.Key)
					default:
						failed++
						note("%s: %v", o.Key, err)
					}
					mu.Unlock()
				}
			}()
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].Key < matches[j].Key })
		for _, o := range matches {
			jobs <- o
		}
		close(jobs)
		wg.Wait()

		verb := "restored"
		if dry {
			verb = "would be restored"
		}
		fmt.Printf("%d files %s", restored, verb)
		if exists > 0 {
			fmt.Printf(", %d already exist (use --overwrite to replace them)", exists)
		}
		fmt.Println()
		for _, p := range problems {
			fmt.Println("  ", p)
		}
		if noContent+failed > 0 {
			return fmt.Errorf("%d files could not be restored", noContent+failed)
		}
		return nil
	},
}

// restoreFile copies one file from the backup into a bucket on the server.
func restoreFile(ctx context.Context, client *apiclient.ApiClient, dir, bucket, key string, o *backupObject, overwrite bool) (string, error) {
	var f *os.File
	if o.Size > 0 || o.SHA256 != "" {
		var err error
		if f, err = backupfile.OpenContent(dir, o.SHA256); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "no-content", err
			}
			return "failed", err
		}
		defer f.Close()
	}
	put := client.PutFileObjectIfAbsent
	if overwrite {
		put = client.PutFileObject
	}
	var body *os.File = f
	var err error
	if body == nil {
		_, err = put(ctx, bucket, key, nil, 0, o.ContentType, o.ModifiedAt)
	} else {
		_, err = put(ctx, bucket, key, body, o.Size, o.ContentType, o.ModifiedAt)
	}
	switch {
	case err == nil:
		return "restored", nil
	case apiclient.IsPreconditionFailed(err):
		return "exists", nil
	}
	return "failed", cmdutil.CleanErr(err)
}

func backupfileManifest(dir string) (*backupfile.Manifest, error) {
	return backupfile.ReadManifest(dir)
}

// listPath lists what a bucket's path names in a backup: a file, or the
// contents of a folder, one level unless recursive. A path ending in / is a
// folder; any other is the file of that name if there is one, else a folder.
func listPath(dir, key, bucketId, prefix string, recursive bool) (folders []string, files []backupObject, err error) {
	var exact *backupObject
	base := prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		base = prefix + "/"
	}
	seen := map[string]bool{}
	err = eachBackupObject(dir, key, func(o *backupObject, _ []byte) error {
		if o.BucketId != bucketId {
			return nil
		}
		if prefix != "" && o.Key == prefix && base != prefix {
			c := *o
			exact = &c
			return nil
		}
		if !strings.HasPrefix(o.Key, base) || o.Key == base { // o.Key == base is a folder's own marker
			return nil
		}
		rest := o.Key[len(base):]
		if !recursive {
			if i := strings.Index(rest, "/"); i >= 0 {
				if f := base + rest[:i+1]; !seen[f] {
					seen[f] = true
					folders = append(folders, f)
				}
				return nil
			}
		}
		files = append(files, *o)
		return nil
	})
	if exact != nil {
		return nil, []backupObject{*exact}, err
	}
	return folders, files, err
}
