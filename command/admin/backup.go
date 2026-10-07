package commands_admin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/backupfile"
	"github.com/paularlott/knot/internal/config"
)

// backupFlags maps the flags that choose what to back up to the kinds of
// record they select.
var backupFlags = []struct {
	flag  string
	kinds []string
}{
	{"templates", []string{"templates"}},
	{"template-vars", []string{"template-vars"}},
	{"volumes", []string{"volumes"}},
	{"groups", []string{"groups"}},
	{"roles", []string{"roles"}},
	{"users", []string{"users"}},
	{"tokens", []string{"tokens"}},
	{"spaces", []string{"spaces"}},
	{"scripts", []string{"scripts"}},
	{"skills", []string{"skills"}},
	{"commands", []string{"commands"}},
	{"responses", []string{"responses"}},
	{"cfg-values", []string{"cfg-values"}},
	{"audit-logs", []string{"audit-logs"}},
	{"files", []string{"file-buckets", "file-objects"}},
}

var BackupCmd = &cli.Command{
	Name:  "backup",
	Usage: "Back up the server to a folder",
	Description: `Back up everything the server holds, while it runs, into a folder: users with their tokens and spaces, templates, groups, roles, scripts, configuration, the audit log, and file storage with every file's content.

The backup is taken through the server's API, by a user holding the Backup Server permission (the Backup User role), so it can run from anywhere. Choose the server with --server and --token, or --alias for one in the configuration file's client.connection section; with neither, the default alias is used.

The folder holds one file of records for each kind, a manifest written when the backup is complete, and file content stored by checksum under content/. Run it again into the same folder to refresh the backup: only content not already there is copied. Records are kept as they were when the backup started; content is copied afterwards, so a file replaced meanwhile may have lost its old content, which is reported.

Selecting kinds backs up only those; with none selected everything is. --encrypt-key encrypts the records, which include password hashes and API tokens; file content is stored as it is, so protect the folder.

Restore the folder into a new server with knot admin restore, and list or restore files from it with knot admin file ls and knot admin file restore.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "backupdir", Usage: "The folder to back up into", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "templates", Aliases: []string{"t"}, Usage: "Backup templates", ConfigPath: []string{"backup.templates"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TEMPLATES"}},
		&cli.BoolFlag{Name: "template-vars", Aliases: []string{"v"}, Usage: "Backup template variables", ConfigPath: []string{"backup.template_vars"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TEMPLATE_VARS"}},
		&cli.BoolFlag{Name: "volumes", Aliases: []string{"l"}, Usage: "Backup volumes", ConfigPath: []string{"backup.volumes"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_VOLUMES"}},
		&cli.BoolFlag{Name: "groups", Aliases: []string{"g"}, Usage: "Backup groups", ConfigPath: []string{"backup.groups"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_GROUPS"}},
		&cli.BoolFlag{Name: "roles", Aliases: []string{"r"}, Usage: "Backup roles", ConfigPath: []string{"backup.roles"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ROLES"}},
		&cli.BoolFlag{Name: "spaces", Aliases: []string{"s"}, Usage: "Backup user spaces", ConfigPath: []string{"backup.spaces"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_SPACES"}},
		&cli.BoolFlag{Name: "users", Aliases: []string{"u"}, Usage: "Backup users", ConfigPath: []string{"backup.users"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_USERS"}},
		&cli.BoolFlag{Name: "tokens", Aliases: []string{"k"}, Usage: "Backup user tokens", ConfigPath: []string{"backup.tokens"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_TOKENS"}},
		&cli.BoolFlag{Name: "cfg-values", Aliases: []string{"o"}, Usage: "Backup configuration values", ConfigPath: []string{"backup.cfg_values"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_CFG_VALUES"}},
		&cli.BoolFlag{Name: "scripts", Aliases: []string{"c"}, Usage: "Backup scripts", ConfigPath: []string{"backup.scripts"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_SCRIPTS"}},
		&cli.BoolFlag{Name: "skills", Aliases: []string{"i"}, Usage: "Backup skills", ConfigPath: []string{"backup.skills"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_SKILLS"}},
		&cli.BoolFlag{Name: "commands", Usage: "Backup slash commands", ConfigPath: []string{"backup.commands"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_COMMANDS"}},
		&cli.BoolFlag{Name: "responses", Aliases: []string{"p"}, Usage: "Backup responses", ConfigPath: []string{"backup.responses"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_RESPONSES"}},
		&cli.BoolFlag{Name: "audit-logs", Usage: "Backup audit logs", ConfigPath: []string{"backup.audit_logs"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_AUDIT_LOGS"}},
		&cli.BoolFlag{Name: "files", Usage: "Backup file storage: buckets, files and their content", ConfigPath: []string{"backup.files"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_FILES"}},
		&cli.BoolFlag{Name: "no-content", Usage: "With file storage, back up the records of buckets and files but not their content.", EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_NO_CONTENT"}},
		&cli.BoolFlag{Name: "prune", Usage: "With file storage and content, remove content from the folder that no file refers to any more, so the folder mirrors the server.", EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_PRUNE"}},
		&cli.BoolFlag{Name: "all", Aliases: []string{"a"}, Usage: "Backup everything", ConfigPath: []string{"backup.all"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ALL"}, DefaultValue: true},
		&cli.StringFlag{Name: "limit-user", Usage: "Limit the backup to a specific user by username.", ConfigPath: []string{"backup.limit_user"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_LIMIT_USER"}},
		&cli.StringFlag{Name: "limit-template", Usage: "Limit the backup to a specific template by name.", ConfigPath: []string{"backup.limit_template"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_LIMIT_TEMPLATE"}},
		&cli.StringFlag{Name: "encrypt-key", Aliases: []string{"e"}, Usage: "Encrypt the records with this key, which must be 32 bytes long.", ConfigPath: []string{"backup.encrypt_key"}, EnvVars: []string{config.CONFIG_ENV_PREFIX + "_BACKUP_ENCRYPT_KEY"}},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		dir := cmd.GetStringArg("backupdir")
		key := cmd.GetString("encrypt-key")
		if key != "" && len(key) != 32 {
			return errors.New("Error: Encrypt key must be 32 bytes long.")
		}

		client, err := adminClient(cmd, false)
		if err != nil {
			return err
		}
		info, err := client.GetBackupInfo(ctx)
		if err != nil {
			return fmt.Errorf("Error starting the backup: %s", cmdutil.CleanAPIError(err))
		}

		// What to back up.
		selected := map[string]bool{}
		explicit := false
		for _, f := range backupFlags {
			if cmd.GetBool(f.flag) {
				explicit = true
				for _, k := range f.kinds {
					selected[k] = true
				}
			}
		}
		if !explicit {
			for _, k := range info.Kinds {
				selected[k] = true
			}
		}
		if !info.Files {
			if selected["file-buckets"] && explicit {
				return errors.New("Error: this server has no file storage to back up.")
			}
			delete(selected, "file-buckets")
			delete(selected, "file-objects")
		}
		if len(selected) == 0 {
			return errors.New("Error: nothing to back up.")
		}
		withContent := selected["file-objects"] && !cmd.GetBool("no-content")

		if err := prepareBackupDir(dir); err != nil {
			return err
		}
		fmt.Printf("Backing up the server (knot %s) to %s\n", info.Version, dir)

		params := url.Values{}
		if v := cmd.GetString("limit-user"); v != "" {
			params.Set("limit_user", v)
		}
		if v := cmd.GetString("limit-template"); v != "" {
			params.Set("limit_template", v)
		}

		counts := map[string]int{}
		var bucketIds map[string]bool
		for _, kind := range apiclient.BackupKinds {
			if !selected[kind] {
				continue
			}
			var keep func(line []byte) bool
			switch kind {
			case "file-buckets":
				keep = func(line []byte) bool {
					var b struct {
						Id string `json:"id"`
					}
					if json.Unmarshal(line, &b) != nil {
						return false
					}
					bucketIds[b.Id] = true
					return true
				}
			case "file-objects":
				// Only the files of the buckets backed up, which were taken
				// at their own moment.
				keep = func(line []byte) bool {
					var o struct {
						BucketId string `json:"bucket_id"`
					}
					return json.Unmarshal(line, &o) == nil && bucketIds[o.BucketId]
				}
			}
			var n int
			err := retry(ctx, func() error {
				bucketIdsReset(kind, &bucketIds)
				var err error
				n, err = backupKind(ctx, client, dir, kind, key, params, keep)
				return err
			})
			if err != nil {
				return fmt.Errorf("Error backing up %s: %w", kind, err)
			}
			counts[kind] = n
			fmt.Printf("  %-14s %d\n", kind, n)
		}

		warnings := 0
		if withContent {
			copied, held, bytes, problems := backupContent(ctx, client, dir, key)
			warnings = len(problems)
			fmt.Printf("  %-14s %d copied (%s), %d already in the backup", "file content", copied, humanBytes(bytes), held)
			if warnings > 0 {
				fmt.Printf(", %d could not be read", warnings)
			}
			fmt.Println()
			for i, p := range problems {
				if i == 10 {
					fmt.Printf("    and %d more\n", len(problems)-10)
					break
				}
				fmt.Println("   ", p)
			}
		}

		if cmd.GetBool("prune") && withContent && warnings == 0 && cmd.GetString("limit-user") == "" {
			removed, freed, err := pruneContent(dir, key)
			if err != nil {
				return fmt.Errorf("Error pruning content: %w", err)
			}
			fmt.Printf("  %-14s %d removed (%s) that no file refers to\n", "pruned", removed, humanBytes(freed))
		}

		if err := backupfile.WriteManifest(dir, &backupfile.Manifest{
			Created: time.Now().UTC(), Server: info.Version, Encrypted: key != "", Counts: counts, Content: withContent,
		}); err != nil {
			return err
		}
		if err := backupfile.ClearUnfinished(dir); err != nil {
			return err
		}
		client.PostBackupComplete(ctx, &apiclient.BackupSummary{Counts: counts, Warnings: warnings})

		if warnings > 0 {
			return fmt.Errorf("the backup is complete except for the content of %d files; they may have been replaced during the backup, or no server holds them", warnings)
		}
		fmt.Println("Backup completed successfully.")
		return nil
	},
}

// bucketIdsReset clears the buckets seen when a retry starts the buckets
// again.
func bucketIdsReset(kind string, ids *map[string]bool) {
	if kind == "file-buckets" {
		*ids = map[string]bool{}
	}
}

// prepareBackupDir makes the folder ready: new, empty, or holding an earlier
// backup, which is marked unfinished until this one completes.
func prepareBackupDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		return backupfile.MarkUnfinished(dir)
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 && !backupfile.IsBackupDir(dir) {
		return fmt.Errorf("Error: %s is not empty and holds no backup; give a new or empty folder", dir)
	}
	// A backup stopped part way must not pass for a complete one: it is marked
	// until this run finishes, and running again carries on from what is there.
	if err := backupfile.MarkUnfinished(dir); err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// backupKind streams the records of a kind into its file, keeping those keep
// accepts (all if nil), and returns how many it kept.
func backupKind(ctx context.Context, client *apiclient.ApiClient, dir, kind, key string, params url.Values, keep func([]byte) bool) (int, error) {
	rc, err := client.BackupStream(ctx, kind, params)
	if err != nil {
		return 0, errors.New(cmdutil.CleanAPIError(err))
	}
	defer rc.Close()

	w, err := backupfile.Create(dir, kind, key)
	if err != nil {
		return 0, err
	}
	err = eachLine(rc, func(line []byte) error {
		if keep != nil && !keep(line) {
			return nil
		}
		return w.Add(line)
	})
	if err != nil {
		w.Close()
		os.Remove(backupfile.RecordPath(dir, kind))
		return 0, err
	}
	n := w.Count()
	return n, w.Close()
}

// eachLine calls fn with each line of r, without its newline.
func eachLine(r io.Reader, fn func(line []byte) error) error {
	br := bufio.NewReaderSize(r, 256*1024)
	for {
		line, err := br.ReadBytes('\n')
		for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
			line = line[:len(line)-1]
		}
		if len(line) > 0 {
			if ferr := fn(line); ferr != nil {
				return ferr
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// contentWorkers is how many files are copied at once.
const contentWorkers = 16

// backupContent copies the content of every file in the backup that the
// folder does not hold yet, from the server. It returns how many it copied,
// how many it already held, the bytes copied and what could not be read.
func backupContent(ctx context.Context, client *apiclient.ApiClient, dir, key string) (copied, held int, total int64, problems []string) {
	r, err := backupfile.Open(dir, "file-objects", key)
	if err != nil {
		return 0, 0, 0, []string{err.Error()}
	}
	defer r.Close()

	var mu sync.Mutex
	jobs := make(chan string, contentWorkers*4)
	var wg sync.WaitGroup
	for i := 0; i < contentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sha := range jobs {
				n, err := copyContent(ctx, client, dir, sha)
				mu.Lock()
				if err != nil {
					problems = append(problems, fmt.Sprintf("%s: %v", sha[:12], err))
				} else {
					copied++
					total += n
				}
				mu.Unlock()
			}
		}()
	}

	var last string
	_ = r.Each(func(line []byte) error {
		var o struct {
			SHA256 string `json:"sha256"`
			Size   int64  `json:"size"`
		}
		if json.Unmarshal(line, &o) != nil || len(o.SHA256) != 64 || o.SHA256 == last {
			return nil
		}
		last = o.SHA256
		if backupfile.HasContentOfSize(dir, o.SHA256, o.Size) {
			mu.Lock()
			held++
			mu.Unlock()
			return nil
		}
		select {
		case jobs <- o.SHA256:
		case <-ctx.Done():
			return ctx.Err()
		}
		return nil
	})
	close(jobs)
	wg.Wait()
	return
}

// copyContent copies one file's content into the backup, retrying a transfer
// that breaks and carrying on from where it stopped.
// pruneContent removes the content in the folder that no file in the backup
// refers to.
func pruneContent(dir, key string) (int, int64, error) {
	wanted := map[string]struct{}{}
	r, err := backupfile.Open(dir, "file-objects", key)
	if err != nil {
		return 0, 0, err
	}
	err = r.Each(func(line []byte) error {
		var o struct {
			SHA256 string `json:"sha256"`
		}
		if json.Unmarshal(line, &o) == nil && len(o.SHA256) == 64 {
			wanted[o.SHA256] = struct{}{}
		}
		return nil
	})
	r.Close()
	if err != nil {
		return 0, 0, err
	}
	return backupfile.Prune(dir, func(sha string) bool { _, ok := wanted[sha]; return ok })
}

func copyContent(ctx context.Context, client *apiclient.ApiClient, dir, sha string) (int64, error) {
	var total int64
	err := retry(ctx, func() error {
		n, err := backupfile.DownloadContent(dir, sha, func(offset int64) (io.ReadCloser, bool, error) {
			resp, resumed, err := client.BackupContent(ctx, sha, offset)
			if err != nil {
				return nil, false, err
			}
			return resp.Body, resumed, nil
		})
		total += n
		return err
	})
	if err != nil {
		return 0, errors.New(cmdutil.CleanAPIError(err))
	}
	return total, nil
}

// attempts is how many times a transfer is tried, with a growing pause
// between, before it is given up.
const attempts = 5

// retry calls fn until it succeeds, tries are exhausted, the context ends or
// the error is one trying again cannot cure: the server refusing the
// request, or content that does not match its checksum.
func retry(ctx context.Context, fn func() error) error {
	var err error
	for try := 0; try < attempts; try++ {
		if try > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(1<<(try-1)) * time.Second):
			}
		}
		if err = fn(); err == nil || !retryable(err) {
			return err
		}
	}
	return err
}

// retryable reports whether an error may pass: a broken connection or a busy
// server, but not a refusal or damaged content.
func retryable(err error) bool {
	msg := err.Error()
	if strings.Contains(msg, "does not match its checksum") {
		return false
	}
	if strings.HasPrefix(msg, "unexpected status code: ") {
		code := strings.TrimPrefix(msg, "unexpected status code: ")
		if len(code) >= 3 {
			switch code[:3] {
			case "408", "429", "500", "502", "503", "504":
				return true
			}
		}
		return false
	}
	return true
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// adminClient connects to the server the user chose, as other client
// commands do. With tokenOptional, a --server given without a token connects
// without one, which is how a server with no users yet is restored into.
func adminClient(cmd *cli.Command, tokenOptional bool) (*apiclient.ApiClient, error) {
	if server := cmd.GetString("server"); tokenOptional && server != "" && cmd.GetString("token") == "" {
		addr := config.NewServerAddr(server, "")
		return apiclient.NewClient(addr.HttpServer, "", cmd.GetBool("tls-skip-verify"))
	}
	return cmdutil.GetClient(cmd)
}
