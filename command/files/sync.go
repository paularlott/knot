package command_files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/excludes"
	"github.com/paularlott/logger"
)

var syncFlags = []cli.Flag{
	&cli.BoolFlag{Name: "delete", Usage: "Delete files at the destination that are not in the source; two ways, pass deletions on to the other side."},
	&cli.BoolFlag{Name: "dry-run", Aliases: []string{"n"}, Usage: "Show what would change without changing anything."},
	&cli.BoolFlag{Name: "watch", Aliases: []string{"w"}, Usage: "Keep running, syncing changes as they are made, until interrupted."},
	&cli.BoolFlag{Name: "two-way", Usage: "Sync both ways: changes on either side are made on the other."},
	&cli.StringSliceFlag{Name: "exclude", Aliases: []string{"x"}, Usage: "Leave out paths matching a pattern, such as node_modules/ or *.log (a trailing / matches folders only). Repeatable."},
	&cli.BoolFlag{Name: "no-default-excludes", Usage: "With --watch or --two-way, sync version control folders and editor scratch files too."},
	&cli.BoolFlag{Name: "allow-mass-delete", Usage: "Delete even when most of a side's files would go, or the other side is empty."},
	&cli.StringFlag{Name: "state-file", Usage: "Two ways, where to keep the sync's state (default: in the user's configuration directory)."},
}

var syncCmd = &cli.Command{
	Name:  "sync",
	Usage: "Make a bucket folder and a local directory, or two bucket folders, match",
	Description: `Copy only what differs from the source to the destination. Either side is a local directory or a bucket folder, bucket:folder (bucket: is a whole bucket), and the direction follows from which is which.

Upload: knot file sync ./site b1:site

Download: knot file sync b1:site ./site

Between buckets, on the server: knot file sync b1:site b2:backup

Files are compared by size and SHA-256 checksum, so unchanged files are never transferred; modification times are kept. With --delete, files at the destination that are not in the source are removed. A file renamed or copied within a bucket is not sent again: its content is copied, or reused if the bucket held it until moments ago.

With --watch the sync keeps running and makes each change as it happens: local changes are seen as they are made, and the bucket's through the server's live updates.

With --two-way, between a directory and a bucket folder, changes on either side are made on the other: knot file sync --two-way --watch ./notes b1:notes. A file changed on both sides since they last matched is kept both ways, the local version renamed to name.conflict-<host>-<time>. Deletions pass to the other side only with --delete; without it a file deleted on one side is put back from the other. The state the two sides last agreed on is kept between runs, in the user's configuration directory or --state-file.

Continual and two-way syncs leave out version control folders (.git, .hg, .svn) and editor and system scratch files unless --no-default-excludes is given; --exclude adds more. A pass that would delete more than half of a side's files (and more than 10), or everything because the other side is empty, is refused, unless --allow-mass-delete is given; while watching, the deletions are held back and the rest goes ahead.

On a file system that ignores case, two bucket files whose names differ only in case cannot both be held: the second is skipped with a warning.`,
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
		return runSyncRequest(ctx, client, cmd.GetStringArg("source"), cmd.GetStringArg("destination"), syncRequest{
			del:               cmd.GetBool("delete"),
			dryRun:            cmd.GetBool("dry-run"),
			watch:             cmd.GetBool("watch"),
			twoWay:            cmd.GetBool("two-way"),
			excludes:          cmd.GetStringSlice("exclude"),
			noDefaultExcludes: cmd.GetBool("no-default-excludes"),
			allowMassDelete:   cmd.GetBool("allow-mass-delete"),
			statePath:         cmd.GetString("state-file"),
		})
	},
}

// syncRequest is what a sync was asked to do.
type syncRequest struct {
	del, dryRun, watch, twoWay bool
	excludes                   []string
	noDefaultExcludes          bool
	allowMassDelete            bool
	statePath                  string
	follow                     eventFollower // the event stream; nil for the server's
}

// runSync makes the destination match the source, in the direction the two
// paths imply.
func runSync(ctx context.Context, client *apiclient.ApiClient, srcArg, dstArg string, del, dryRun bool) error {
	return runSyncRequest(ctx, client, srcArg, dstArg, syncRequest{del: del, dryRun: dryRun})
}

func runSyncRequest(ctx context.Context, client *apiclient.ApiClient, srcArg, dstArg string, req syncRequest) error {
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
	if req.dryRun && req.watch {
		return fmt.Errorf("--dry-run shows a single pass: it cannot be combined with --watch")
	}
	if req.statePath != "" && !req.twoWay {
		return fmt.Errorf("--state-file is for --two-way")
	}

	switch {
	case src.remote && dst.remote:
		if req.watch || req.twoWay {
			return fmt.Errorf("--watch and --two-way sync a local directory and a bucket folder, not two bucket folders")
		}
		return syncBuckets(ctx, client, src.bucket, dirPrefix(src.key), dst.bucket, dirPrefix(dst.key), req.del, req.dryRun)
	case !src.remote && !dst.remote:
		return fmt.Errorf("%s and %s are both local: one side must be a bucket path, written bucket:folder", src, dst)
	}

	opt := syncOptions{
		mode:            modeUp,
		del:             req.del,
		dryRun:          req.dryRun,
		allowMassDelete: req.allowMassDelete,
		statePath:       req.statePath,
		watching:        req.watch,
	}
	local, remote := src, dst
	if src.remote {
		opt.mode, local, remote = modeDown, dst, src
	}
	if req.twoWay {
		opt.mode = modeTwoWay
	}
	opt.dir, err = filepath.Abs(local.raw)
	if err != nil {
		return err
	}
	opt.bucket, opt.prefix = remote.bucket, dirPrefix(remote.key)
	patterns := req.excludes
	if (req.watch || req.twoWay) && !req.noDefaultExcludes {
		patterns = append(append([]string{}, excludes.Defaults...), patterns...)
	}
	opt.excludes = excludes.Compile(patterns)

	return runEngine(ctx, client, opt, req.follow)
}

// runEngine runs a sync between a directory and a bucket folder: one pass,
// or, watching, a pass and then one for each change.
func runEngine(ctx context.Context, client *apiclient.ApiClient, opt syncOptions, follow eventFollower) error {
	s := newSyncer(client, opt)
	if err := s.prepare(ctx); err != nil {
		return err
	}
	// The change feed gives a cursor to follow the bucket by; a single pass
	// lists it.
	if err := s.loadRemote(ctx, opt.watching); err != nil {
		return err
	}
	// A directory a dry run down would make does not exist yet.
	if _, err := os.Stat(s.opt.dir); err == nil {
		if err := s.scanLocal(); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	err := s.pass(ctx, nil)
	s.report()
	if !opt.watching {
		return err
	}
	if err != nil {
		s.log.WithError(err).Warn("sync pass incomplete")
	}
	if follow == nil {
		follow = client.FollowEvents
	}
	return s.watch(ctx, follow)
}

// syncUp and syncDown are single passes up and down.
func syncUp(ctx context.Context, client *apiclient.ApiClient, dir, bucket, prefix string, del, dryRun bool) error {
	return runEngine(ctx, client, syncOptions{mode: modeUp, dir: dir, bucket: bucket, prefix: prefix, del: del, dryRun: dryRun}, nil)
}

func syncDown(ctx context.Context, client *apiclient.ApiClient, bucket, prefix, dir string, del, dryRun bool) error {
	return runEngine(ctx, client, syncOptions{mode: modeDown, dir: dir, bucket: bucket, prefix: prefix, del: del, dryRun: dryRun}, nil)
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

func (c syncCounts) report(l logger.Logger, dryRun bool) {
	msg := "synced"
	if dryRun {
		msg = "dry run: nothing changed"
	}
	l.Info(msg, "copied", c.copied, "unchanged", c.unchanged, "deleted", c.deleted)
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

	l := log.WithGroup("sync")
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
		from, to := remoteName(srcBucket, o.Key), remoteName(dstBucket, dstPrefix+rel)
		if dryRun {
			l.Info("would copy", "file", from, "to", to)
			continue
		}
		info, err := client.CopyFileObject(ctx, apiclient.FileCopyRequest{SourceBucket: srcBucket, SourceKey: o.Key, DestBucket: dstBucket, DestKey: dstPrefix + rel})
		if err != nil {
			return fmt.Errorf("%s: %s", from, cmdutil.CleanAPIError(err))
		}
		l.Info("copied", "file", from, "to", to, "size", formatBytes(info.Size))
	}

	if del {
		for rel := range dst {
			if _, ok := src[rel]; ok || rel == "" {
				continue
			}
			counts.deleted++
			if dryRun {
				l.Info("would delete", "file", remoteName(dstBucket, dstPrefix+rel))
				continue
			}
			if err := client.DeleteFileObject(ctx, dstBucket, dstPrefix+rel); err != nil {
				return fmt.Errorf("%s: %s", remoteName(dstBucket, dstPrefix+rel), cmdutil.CleanAPIError(err))
			}
			l.Info("deleted", "file", remoteName(dstBucket, dstPrefix+rel))
		}
	}

	counts.report(l, dryRun)
	return nil
}
