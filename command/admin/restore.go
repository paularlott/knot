package commands_admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/backupfile"
	"github.com/paularlott/knot/internal/config"
)

var RestoreCmd = &cli.Command{
	Name:  "restore",
	Usage: "Restore a backup into a server",
	Description: `Restore a backup folder made by knot admin backup into a running server, which is how a lost server is rebuilt: install knot, start a new server, and restore into it.

Restoring needs a token from a user holding the Backup Server permission. To rebuild a lost server, start a new one, create its first user (it takes the Admin and Backup User roles), and restore with that user's token. Use a username and email the backup doesn't contain, as the backup's own users are restored over it, and delete that user afterwards. The users come last, so a failed restore can simply be run again.

Records are saved over any the server already holds; file records keep their timestamps, so a newer version of a bucket or file already on the server is kept. File content is uploaded for files whose content the server doesn't hold. Use --no-content to restore the records only.

To get back a single file or folder, rather than the whole backup, use knot admin file restore.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "backupdir", Usage: "The backup folder", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Flags: []cli.Flag{
		&cli.StringFlag{Name: "encrypt-key", Aliases: []string{"e"}, Usage: "The key the records were encrypted with.", EnvVars: []string{config.CONFIG_ENV_PREFIX + "_RESTORE_ENCRYPT_KEY"}},
		&cli.BoolFlag{Name: "no-content", Usage: "Restore the records of buckets and files but not their content."},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		dir := cmd.GetStringArg("backupdir")
		key := cmd.GetString("encrypt-key")

		manifest, err := backupfile.ReadManifest(dir)
		if err != nil {
			return err
		}
		if manifest.Encrypted && key == "" {
			return errors.New("the backup is encrypted: give the key with --encrypt-key")
		}
		client, err := adminClient(cmd)
		if err != nil {
			return err
		}

		fmt.Printf("Restoring the backup of %s (knot %s) from %s\n", manifest.Created.Local().Format("2006-01-02 15:04:05"), manifest.Server, dir)
		summary := &apiclient.BackupSummary{Counts: map[string]int{}}
		problems := 0

		for _, kind := range apiclient.BackupKinds {
			if _, ok := manifest.Counts[kind]; !ok {
				continue
			}
			var res *apiclient.RestoreResult
			var sent int
			err := retry(ctx, func() error {
				var err error
				res, sent, err = restoreKind(ctx, client, dir, kind, key)
				return err
			})
			if err != nil {
				return fmt.Errorf("couldn't restore the %s records: %w", kind, err)
			}
			summary.Counts[kind] = res.Restored
			fmt.Printf("  %-14s %d restored", kind, res.Restored)
			if res.Skipped > 0 {
				fmt.Printf(", %d could not be", res.Skipped)
				problems += res.Skipped
			}
			fmt.Println()
			for _, e := range res.Errors {
				fmt.Println("   ", e)
			}
			if sent != manifest.Counts[kind] {
				fmt.Printf("    the backup lists %d records, %d were read: the file may be damaged\n", manifest.Counts[kind], sent)
				problems++
			}

			if kind == "file-objects" && manifest.Content && !cmd.GetBool("no-content") {
				uploaded, held, missing := restoreContent(ctx, client, dir, key)
				fmt.Printf("  %-14s %d uploaded, %d already held", "file content", uploaded, held)
				if len(missing) > 0 {
					fmt.Printf(", %d not in the backup", len(missing))
					problems += len(missing)
				}
				fmt.Println()
				for i, m := range missing {
					if i == 10 {
						fmt.Printf("    and %d more\n", len(missing)-10)
						break
					}
					fmt.Println("   ", m)
				}
			}
		}

		summary.Warnings = problems
		// Recording the end is best effort; the restore itself is done.
		client.PostRestoreComplete(ctx, summary)

		if problems > 0 {
			return fmt.Errorf("the restore finished with %d problems", problems)
		}
		fmt.Println("Restore completed successfully.")
		return nil
	},
}

// restoreKind sends the records of a kind to the server, returning what it
// did and how many records it sent.
func restoreKind(ctx context.Context, client *apiclient.ApiClient, dir, kind, key string) (*apiclient.RestoreResult, int, error) {
	r, err := backupfile.Open(dir, kind, key)
	if err != nil {
		return nil, 0, err
	}
	defer r.Close()

	pr, pw := io.Pipe()
	defer pr.Close()
	sent := make(chan int, 1)
	go func() {
		n := 0
		err := r.Each(func(line []byte) error {
			n++
			_, err := pw.Write(append(line, '\n'))
			return err
		})
		sent <- n
		pw.CloseWithError(err)
	}()

	res, err := client.RestoreStream(ctx, kind, pr)
	if err != nil {
		return nil, 0, cmdutil.CleanErr(err)
	}
	return res, <-sent, nil
}

// restoreBatch is how many checksums are asked about at once.
const restoreBatch = 1000

// restoreContent uploads the content the server lacks, returning how many it
// sent, how many it already held, and the files whose content the backup
// does not have.
func restoreContent(ctx context.Context, client *apiclient.ApiClient, dir, key string) (uploaded, held int, missing []string) {
	r, err := backupfile.Open(dir, "file-objects", key)
	if err != nil {
		return 0, 0, []string{err.Error()}
	}
	defer r.Close()

	var mu sync.Mutex
	jobs := make(chan string, contentWorkers*2)
	var wg sync.WaitGroup
	for i := 0; i < contentWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sha := range jobs {
				err := uploadContent(ctx, client, dir, sha)
				mu.Lock()
				if err != nil {
					missing = append(missing, fmt.Sprintf("%s: %v", sha[:12], err))
				} else {
					uploaded++
				}
				mu.Unlock()
			}
		}()
	}

	batch := map[string]bool{}
	flush := func() {
		if len(batch) == 0 {
			return
		}
		shas := make([]string, 0, len(batch))
		for sha := range batch {
			shas = append(shas, sha)
		}
		batch = map[string]bool{}
		need, err := client.RestoreMissingContent(ctx, shas)
		if err != nil {
			mu.Lock()
			missing = append(missing, "asking the server what it lacks: "+cmdutil.CleanAPIError(err))
			mu.Unlock()
			return
		}
		mu.Lock()
		held += len(shas) - len(need)
		mu.Unlock()
		for _, sha := range need {
			jobs <- sha
		}
	}
	_ = r.Each(func(line []byte) error {
		var o struct {
			SHA256 string `json:"sha256"`
		}
		if json.Unmarshal(line, &o) == nil && len(o.SHA256) == 64 {
			batch[o.SHA256] = true
			if len(batch) >= restoreBatch {
				flush()
			}
		}
		return ctx.Err()
	})
	flush()
	close(jobs)
	wg.Wait()
	return
}

func uploadContent(ctx context.Context, client *apiclient.ApiClient, dir, sha string) error {
	if !backupfile.HasContent(dir, sha) {
		return errors.New("not in the backup")
	}
	return retry(ctx, func() error { return uploadContentOnce(ctx, client, dir, sha) })
}

func uploadContentOnce(ctx context.Context, client *apiclient.ApiClient, dir, sha string) error {
	f, err := backupfile.OpenContent(dir, sha)
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("not in the backup")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if err := client.RestoreContent(ctx, sha, f, info.Size()); err != nil {
		return cmdutil.CleanErr(err)
	}
	return nil
}
