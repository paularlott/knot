package command_files

import (
	"context"
	"fmt"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
)

var snapshotCmd = &cli.Command{
	Name:      "snapshot",
	Usage:     "Copy a folder, as it is at one moment, to a new folder",
	Description: `Copy a bucket folder and everything below it to a new folder, in the same bucket or another, as the folder was at one moment: however much is being written to it, the copy holds every file as it was when the snapshot was taken, never some files older or newer than the rest.

knot file snapshot b1:src/ b1:snapshots/2026-11-06/

The whole bucket: knot file snapshot b1: b2:backup/

The copy is made on the server and no content is transferred, but the new files count towards the destination bucket owner's storage, as copies do. The new folder must not already exist. Its files appear over a moment; the snapshot is complete once its manifest, ` + "`.knot-snapshot.json`" + `, is there. A snapshot takes what the server holds: files still being saved, from a mount or a sync, are not in it until they have been saved.`,
	Arguments: []cli.Argument{
		&cli.StringArg{Name: "folder", Usage: "The folder to snapshot, as bucket:path (bucket: for the whole bucket)", Required: true},
		&cli.StringArg{Name: "new-folder", Usage: "The folder to make, as bucket:path", Required: true},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		client, err := getClient(cmd)
		if err != nil {
			return err
		}
		return runSnapshot(ctx, client, cmd.GetStringArg("folder"), cmd.GetStringArg("new-folder"))
	},
}

func runSnapshot(ctx context.Context, client *apiclient.ApiClient, from, to string) error {
	src, err := requireRemote(from)
	if err != nil {
		return err
	}
	dst, err := requireRemote(to)
	if err != nil {
		return err
	}
	if hasGlob(src.key) {
		return fmt.Errorf("%s: a snapshot is of one folder, not a pattern", from)
	}
	if strings.Trim(dst.key, "/") == "" {
		return fmt.Errorf("%s: give the new folder to make, e.g. %s:snapshots/today/", to, dst.bucket)
	}

	snap, err := client.SnapshotFileFolder(ctx, apiclient.FileSnapshotRequest{
		SourceBucket: src.bucket,
		Source:       src.key,
		DestBucket:   dst.bucket,
		Dest:         dst.key,
	})
	if err != nil {
		return fmt.Errorf("snapshot of %s: %w", from, apiError(err))
	}
	newFolder := remoteName(dst.bucket, strings.TrimSuffix(snap.Manifest, ".knot-snapshot.json"))
	fmt.Printf("%s -> %s: %d files in %d folders (%s), as at %s\n", from, newFolder, snap.Files, snap.Folders, formatBytes(snap.Bytes), snap.TakenAt.Local().Format("2006-01-02 15:04:05"))
	return nil
}
