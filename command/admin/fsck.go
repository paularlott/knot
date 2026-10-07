package commands_admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/util"
)

func printJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

var FsckCmd = &cli.Command{
	Name:  "fsck",
	Usage: "Check file storage on the running server for damage, and repair it",
	Description: `Check a running server's file storage for damage: file content that is missing or wrong, reference counts and indexes that disagree with the file records, statistics that have drifted, and buckets whose owner was deleted.

With --repair, content that is missing or wrong is fetched from the other servers in the cluster, counts, indexes and statistics are rebuilt from the file records, and buckets of deleted users are removed. A file whose content no server holds can't be repaired from the cluster: add --from-backup to restore it from a backup folder made by knot admin backup, or delete the file. Records of files are never removed by a check.

--deep also reads every stored file and checks its checksum, which takes a while on a large store. Writes pause briefly while counts are rebuilt, and only if they were wrong.

The check needs the Manage File Storage permission; --from-backup also needs the Backup Server permission. Choose the server with --server and --token, or --alias for one in the configuration file; with neither, the default alias is used. The exit status is non-zero while problems remain.`,
	Flags: []cli.Flag{
		&cli.BoolFlag{Name: "repair", Usage: "Fix what can be fixed."},
		&cli.BoolFlag{Name: "deep", Usage: "Check the checksum of every stored file."},
		&cli.StringFlag{Name: "from-backup", Usage: "With --repair, restore content no server holds from this backup folder."},
		&cli.BoolFlag{Name: "json", Usage: "Print the report as JSON."},
		encryptKeyFlag,
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		opts := filestore.FsckOptions{Repair: cmd.GetBool("repair"), Deep: cmd.GetBool("deep")}
		backup := cmd.GetString("from-backup")
		if backup != "" && !opts.Repair {
			return errors.New("Error: --from-backup repairs, so it needs --repair.")
		}
		key := cmd.GetString("encrypt-key")
		if backup != "" {
			m, err := backupfileManifest(backup)
			if err != nil {
				return fmt.Errorf("Error: %w", err)
			}
			if m.Encrypted && key == "" {
				return errors.New("Error: the backup is encrypted: give the key with --encrypt-key.")
			}
			if !m.Content {
				return errors.New("Error: the backup holds no file content.")
			}
		}

		client, err := adminClient(cmd)
		if err != nil {
			return err
		}
		report := &filestore.FsckReport{}
		if err := client.FilesFsck(ctx, opts, report); err != nil {
			return fmt.Errorf("Error checking file storage: %s", cmdutil.CleanAPIError(err))
		}

		// Content no server holds comes from the backup.
		var restored, held int
		var notInBackup []string
		if backup != "" && report.Found[filestore.FsckMissingContent]+report.Found[filestore.FsckCorruptContent] > 0 {
			restored, held, notInBackup = restoreContent(ctx, client, backup, key)
			report.Repaired[filestore.FsckMissingContent] = min(report.Found[filestore.FsckMissingContent], report.Repaired[filestore.FsckMissingContent]+restored)
		}

		if cmd.GetBool("json") {
			if err := printJSON(report); err != nil {
				return err
			}
		} else {
			printFsck(report, opts)
			if backup != "" {
				fmt.Printf("From the backup: %d contents restored, %d already held", restored, held)
				if len(notInBackup) > 0 {
					fmt.Printf(", %d not in the backup", len(notInBackup))
				}
				fmt.Println()
			}
		}
		if n := report.Unresolved(); n > 0 {
			return fmt.Errorf("%d problems remain", n)
		}
		return nil
	},
}

func printFsck(r *filestore.FsckReport, opts filestore.FsckOptions) {
	fmt.Printf("Checked %d buckets holding %d files\n", r.Buckets, r.Files)
	if len(r.Found) == 0 {
		fmt.Println("No problems found")
	} else {
		kinds := make([]string, 0, len(r.Found))
		for k := range r.Found {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		table := [][]string{{"PROBLEM", "FOUND", "REPAIRED"}}
		for _, k := range kinds {
			table = append(table, []string{k, fmt.Sprint(r.Found[k]), fmt.Sprint(r.Repaired[k])})
		}
		fmt.Println()
		util.PrintTable(table)
		fmt.Println()

		shown := r.Issues
		if len(shown) > 25 {
			shown = shown[:25]
		}
		for _, i := range shown {
			where := i.Bucket
			if i.Key != "" {
				where += ":" + i.Key
			}
			if where != "" {
				where += " "
			}
			fmt.Printf("  %s: %s%s\n", i.Kind, where, i.Detail)
		}
		if n := len(r.Issues) - len(shown); n > 0 {
			fmt.Printf("  and %d more (--json lists up to %d)\n", n, len(r.Issues))
		}
		if !opts.Repair && r.Unresolved() > 0 {
			fmt.Println("\nRun again with --repair to fix what can be fixed")
		}
	}
	for _, n := range r.Notes {
		fmt.Println("Note:", n)
	}
}
