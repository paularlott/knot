package command_spaces

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"hash/crc64"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util/excludes"
)

// MirrorCmd mirrors a directory tree between this machine and a space, in
// whichever direction the arguments give: the side written <space>:<path>
// is the space. Copies every changed file (preserving each file's mtime and
// permission bits) and deletes any file at the destination that the source
// doesn't have — the destination ends up as a mirror of the source.
//
//	knot space mirror ./src myspace:/var/www/html   # this machine → space
//	knot space mirror myspace:/var/www/html ./src   # space → this machine
//
// For one-way upload without deletes, use knot space copy per-file or write
// your own loop. For continuous two-way sync, mutagen against the space's SSH
// endpoint is the recommendation; mirror is designed for one-shot publishing
// of a tree.
var MirrorCmd = &cli.Command{
	Name:        "mirror",
	Usage:       "Mirror a directory between this machine and a space",
	Description: "Copy <source> to <destination>, then delete files at the destination that the source doesn't have. The destination ends up as a mirror of the source. Write the space side as <space>:<path>: `knot space mirror ./site myspace:/var/www` uploads, and `knot space mirror myspace:/var/www ./site` downloads. With --watch, keeps running and uploads local changes live. For one-shot copies without deletes, use `knot space copy` per-file.",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "exclude",
			Aliases: []string{"x"},
			Usage:   "Glob patterns to skip (e.g. node_modules, *.log). Repeatable.",
		},
		&cli.IntFlag{
			Name:         "parallel",
			Usage:        "Files copied at once (default 8)",
			DefaultValue: 8,
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "List what would be copied or deleted without changing anything",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Log why each file was copied (new / size differs / mtime drift) — for diagnosing idempotence issues",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "Print every copy and delete as it happens (default: summary only)",
		},
		&cli.BoolFlag{
			Name:  "verify",
			Usage: "Compare local and remote hashes without copying or deleting; report any mismatches",
		},
		&cli.BoolFlag{
			Name:  "hash",
			Usage: "Use crc64 hash comparison instead of mtime+size (slower but definitive — catches content drift that mtime misses)",
		},
		&cli.BoolFlag{
			Name:  "watch",
			Usage: "After the initial mirror, watch for local changes and upload them live (uploads only). Ctrl+C to stop",
		},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "source",
			Required: true,
			Usage:    "Folder to mirror: a local path, or 'space:path'",
		},
		&cli.StringArg{
			Name:     "destination",
			Required: true,
			Usage:    "Where to mirror it: 'space:path', or a local path",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		source := cmd.GetStringArg("source")
		destination := cmd.GetStringArg("destination")

		local, remote, pull := destination, source, true
		switch sourceInSpace, destInSpace := isSpacePath(source), isSpacePath(destination); {
		case sourceInSpace && destInSpace:
			return fmt.Errorf("mirror copies between this machine and a space, not between two spaces")
		case !sourceInSpace && !destInSpace:
			return fmt.Errorf("give the space side as 'space:path'")
		case destInSpace:
			local, remote, pull = source, destination, false
		}

		colon := strings.Index(remote, ":")
		spaceName := remote[:colon]
		remoteDir := remote[colon+1:]
		if remoteDir == "" {
			return fmt.Errorf("remote space path cannot be empty after '%s:'", spaceName)
		}

		info, err := os.Stat(local)
		switch {
		case err == nil && !info.IsDir():
			return fmt.Errorf("local path must be a directory (got %q)", local)
		case err != nil && !(pull && os.IsNotExist(err)):
			// Downloading creates the local folder; uploading needs it.
			return fmt.Errorf("stat local: %w", err)
		}
		if pull && cmd.GetBool("watch") {
			return fmt.Errorf("--watch only works when uploading to a space")
		}
		if cmd.GetBool("verify") && cmd.GetBool("watch") {
			return fmt.Errorf("--verify only checks and changes nothing, so it can't be used with --watch")
		}

		client, err := cmdutil.GetClient(cmd)
		if err != nil {
			return err
		}
		spaceID, err := resolveSpaceID(ctx, client, spaceName)
		if err != nil {
			return err
		}

		// File writes can take a while for large files (databases, media,
		// big logs). The apiclient default client timeout is 10s; for the
		// whole mirror run, disable it and rely on the caller's context
		// (Ctrl+C / SIGTERM) for cancellation. Restored on exit so a future
		// command on the same client (shouldn't happen in CLI but defensive)
		// isn't affected.
		client.SetTimeout(0)
		defer client.SetTimeout(10 * time.Second)

		opts := mirrorOptions{
			client:    client,
			spaceID:   spaceID,
			spaceName: spaceName,
			localRoot: local,
			remoteDir: path.Clean(remoteDir),
			excludes:  cmd.GetStringSlice("exclude"),
			parallel:  cmd.GetInt("parallel"),
			dryRun:    cmd.GetBool("dry-run"),
			debug:     cmd.GetBool("debug"),
			// --debug implies --verbose: skip-reason lines are meaningless
			// without the upload lines for context.
			verbose: cmd.GetBool("verbose") || cmd.GetBool("debug"),
			verify:  cmd.GetBool("verify"),
			hash:    cmd.GetBool("hash") || cmd.GetBool("verify"), // verify always uses hash
			watch:   cmd.GetBool("watch"),
			pull:    pull,
		}
		if opts.parallel < 1 {
			opts.parallel = 1
		}
		if opts.parallel > 32 {
			fmt.Fprintf(os.Stderr, "Note: --parallel %d capped to 32 (server connection limit)\n", opts.parallel)
			opts.parallel = 32
		}

		switch {
		case opts.verify:
			fmt.Fprintf(os.Stderr, "Verifying %s ↔ %s:%s\n", local, spaceName, remoteDir)
		case pull:
			fmt.Fprintf(os.Stderr, "Mirroring %s:%s → %s\n", spaceName, remoteDir, local)
		default:
			fmt.Fprintf(os.Stderr, "Mirroring %s → %s:%s\n", local, spaceName, remoteDir)
		}

		if opts.watch {
			return opts.runWatch(ctx)
		}

		start := time.Now()
		run := opts.run
		if pull {
			run = opts.runPull
		}
		stats, err := run(ctx)
		if err != nil {
			return err
		}
		if !opts.verify {
			fmt.Fprintf(os.Stderr, "Done: %s\n", stats.summary(time.Since(start), pull))
		}
		if failed := stats.failed.Load(); failed > 0 {
			if pull {
				return fmt.Errorf("%d file(s) couldn't be mirrored from %s:%s", failed, spaceName, remoteDir)
			}
			return fmt.Errorf("%d file(s) couldn't be mirrored to %s:%s", failed, spaceName, remoteDir)
		}
		return nil
	},
}

// mirrorStats tracks one cycle's counts. All atomic — workers mutate
// concurrently. Honest counts even when individual ops fail (the cycle
// keeps going).
type mirrorStats struct {
	scanned      atomic.Int64
	uploaded     atomic.Int64 // or downloaded, when pulling
	skipped      atomic.Int64
	deleted      atomic.Int64
	failed       atomic.Int64
	bytes        atomic.Int64
	hashSkipped  atomic.Int64 // skipped via definitive hash comparison
	mtimeSkipped atomic.Int64 // skipped via mtime fallback (agent too old for hashing)
}

func (s *mirrorStats) String(duration time.Duration) string {
	return s.summary(duration, false)
}

// summary reports the cycle, naming the copies for their direction.
func (s *mirrorStats) summary(duration time.Duration, pull bool) string {
	skipped := s.skipped.Load()
	detail := ""
	if skipped > 0 {
		h := s.hashSkipped.Load()
		m := s.mtimeSkipped.Load()
		if h > 0 || m > 0 {
			detail = fmt.Sprintf(" (%d hash, %d mtime)", h, m)
		}
	}
	copied := "uploaded"
	if pull {
		copied = "downloaded"
	}
	return fmt.Sprintf("%d scanned, %d %s, %d skipped%s, %d deleted, %d failed (%s)",
		s.scanned.Load(), s.uploaded.Load(), copied, skipped, detail,
		s.deleted.Load(), s.failed.Load(), duration.Round(time.Millisecond))
}

// chunkSize is the threshold for chunked uploads. Files larger than this are
// uploaded in chunkSize-byte blocks: the first block uses mode="overwrite",
// subsequent blocks use mode="append". The last block carries the mtime +
// permission bits for applySyncMetadata on the agent side.
//
// Sized to match the yamux MaxStreamWindowSize (4MB) so each chunk's msgpack
// message fits in one flow-control window — one round-trip through the
// server→agent transport instead of many. A var (not const) so tests can
// shrink it to exercise the chunked path without writing hundreds of
// megabytes.
var chunkSize = int64(4 * 1024 * 1024) // 4 MB — matches yamux stream window

// mtimeTolerance is the fallback window used only when the agent didn't
// return a hash (Hash=0). When hashes are available, comparison is
// definitive and this tolerance is irrelevant.
const mtimeTolerance = 1 * time.Second

// crc64ISO is the table used for local file hashing. Must match the agent's
// hashFile in extlibs/filesystem/find.go (crc64.ISO).
var crc64ISO = crc64.MakeTable(crc64.ISO)

// hashLocalFile computes crc64-ISO of a local file. Used for the definitive
// skip comparison against the agent's hash. Returns 0 on error (which forces
// a re-upload — safe).
func hashLocalFile(path string) uint64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	h := crc64.New(crc64ISO)
	if _, err := io.Copy(h, f); err != nil {
		return 0
	}
	return h.Sum64()
}

// mirrorOptions is the pure-logic payload for one mirror cycle. Decoupled
// from *cli.Command so tests can call it directly with a fake client.
type mirrorOptions struct {
	client    mirrorClient
	spaceID   string
	spaceName string
	localRoot string
	remoteDir string
	excludes  []string
	parallel  int
	dryRun    bool
	debug     bool
	verbose   bool
	verify    bool // hash-only comparison, no uploads/deletes
	hash      bool // use crc64 comparison instead of mtime+size
	watch     bool // after initial mirror, watch for changes
	pull      bool // space → local rather than local → space

	// onEntry, when set, is told of each local path a push's walk sees, and
	// onFail of each path it could not send (deleting: a remote path it
	// could not remove), in place of printing it. --watch uses them to know
	// what the space was given. onFail may be called from several goroutines.
	onEntry func(rel string, info os.FileInfo, target string)
	onFail  func(rel string, deleting bool, err error)
}

// failed counts and reports a path a push could not send.
func (o *mirrorOptions) failed(stats *mirrorStats, rel string, deleting bool, err error) {
	stats.failed.Add(1)
	if o.onFail != nil {
		o.onFail(rel, deleting, err)
		return
	}
	fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, cmdutil.Describe(err))
}

// mirrorClient is the subset of *apiclient.ApiClient that mirror needs.
// *apiclient.ApiClient satisfies it implicitly.
type mirrorClient interface {
	Find(ctx context.Context, spaceID string, req apiclient.FindRequest) (*apiclient.FindResponse, error)
	WriteSpaceFileOpts(ctx context.Context, spaceID, filePath, content, mode string, mtimeNs int64, filePerm uint32) error
	DeleteSpaceFile(ctx context.Context, spaceID string, req apiclient.DeleteFileRequest) (*apiclient.DeleteFileResponse, error)
	CreateSymlinkSpaceFile(ctx context.Context, spaceID, filePath, target string) error
	MakeSpaceDir(ctx context.Context, spaceID, dirPath string, perm uint32) error
	ChmodSpaceFile(ctx context.Context, spaceID, filePath string, perm uint32) error
	RenameSpaceFile(ctx context.Context, spaceID, from, filePath string, mtimeNs int64, filePerm uint32) error
	ReadSpaceFileBytes(ctx context.Context, spaceID, filePath string, offset, length int64) (*apiclient.SpaceFileRange, error)
}

// remoteMeta is the subset of FindEntry that mirror compares against local
// files to decide whether an upload can be skipped.
type remoteMeta struct {
	size       int64
	mtime      time.Time
	isDir      bool
	hash       uint64
	linkTarget string // symlink target; empty for regular files
	perm       uint32 // permission bits; 0 when the agent gave none
}

// upload is the per-file work item. Value semantics so it's safe to pass
// across goroutines without aliasing.
type upload struct {
	rel           string
	localAbs      string
	size          int64
	mtime         time.Time
	mode          uint32
	symlinkTarget string // when non-empty, create symlink instead of writing content
}

// run executes one mirror cycle: fetch the remote path+metadata map, walk
// local, skip files that already match (size + mtime ±1s), upload the rest
// in parallel, then delete any remote path not present locally. The cycle
// is atomic from the caller's POV — failures of individual files don't
// abort the run.
func (o *mirrorOptions) run(ctx context.Context) (*mirrorStats, error) {
	stats := &mirrorStats{}
	o.resolveRoot()

	// Fetch remote set WITH metadata + hash. The per-entry stat and hash on
	// the remote is cheap compared to re-uploading unchanged files.
	remoteSet, err := o.fetchRemoteSet(ctx)
	if err != nil {
		if o.hash && strings.Contains(err.Error(), "503") {
			return stats, fmt.Errorf("list remote: %w\n\nThe agent timed out hashing the remote tree (common for large spaces with --hash or --verify).\nTry without --hash — the default mtime+size comparison is fast and sufficient for most cases", err)
		}
		return stats, fmt.Errorf("list remote: %w", err)
	}

	// Compile excludes early so we can strip excluded paths from the remote
	// set before any comparison. Excluded paths must never be compared,
	// uploaded, or deleted — they're invisible to mirror.
	excludes := compileExcludes(o.excludes)
	for rel, meta := range remoteSet {
		if excludes(rel, meta.isDir) {
			delete(remoteSet, rel)
		}
	}

	// --verify: hash-only comparison, no uploads or deletes.
	if o.verify {
		return o.runVerify(ctx, remoteSet, stats)
	}

	// Walk local, collecting regular files for batch processing.
	// Symlinks and directories are handled inline (they don't need hashing).
	var uploads []upload
	type collectedFile struct {
		rel      string
		localAbs string
		size     int64
		mtime    time.Time
		mode     uint32
	}
	var collected []collectedFile
	// Paths the space has as another kind than here (a folder where this is
	// a file, a symlink where this is a file): removed before the uploads,
	// so a file is never written through an old symlink or onto a folder.
	var conflicts []string
	// Folders, and those holding something mirror sends: the space makes
	// the folders files go into, the rest (empty ones, and those of another
	// mode) are made after the uploads.
	var dirs []localDir
	var chmods []localDir // files whose mode alone changed
	holds := map[string]bool{}

	walkErr := filepath.WalkDir(o.localRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if p == o.localRoot {
			return nil
		}
		rel, err := filepath.Rel(o.localRoot, p)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if excludes(relSlash, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			r, onRemote := remoteSet[relSlash]
			if onRemote && !r.isDir {
				conflicts = append(conflicts, relSlash)
				onRemote = false
			}
			delete(remoteSet, relSlash)
			holds[path.Dir(relSlash)] = true
			if info, err := d.Info(); err == nil {
				dirs = append(dirs, localDir{rel: relSlash, perm: dirPerm(info), onRemote: onRemote, remotePerm: r.perm})
				if o.onEntry != nil {
					o.onEntry(relSlash, info, "")
				}
			}
			return nil
		}
		// Symlink: read target, compare against remote, queue or skip.
		if d.Type()&os.ModeSymlink != 0 {
			stats.scanned.Add(1)
			target, err := os.Readlink(p)
			if err != nil {
				return nil
			}
			holds[path.Dir(relSlash)] = true
			if o.onEntry != nil {
				if info, err := d.Info(); err == nil {
					o.onEntry(relSlash, info, target)
				}
			}
			r, hadRemote := remoteSet[relSlash]
			delete(remoteSet, relSlash)
			if hadRemote && r.isDir {
				conflicts = append(conflicts, relSlash)
			} else if hadRemote && r.linkTarget == target {
				stats.skipped.Add(1)
				return nil
			}
			if o.debug {
				if !hadRemote {
					fmt.Fprintf(os.Stderr, "  ? %s — new symlink (→ %s)\n", relSlash, target)
				} else {
					fmt.Fprintf(os.Stderr, "  ? %s — symlink target differs (local=%s remote=%s)\n", relSlash, target, r.linkTarget)
				}
			}
			uploads = append(uploads, upload{rel: relSlash, localAbs: p, symlinkTarget: target})
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}
		// Skip sockets, pipes, devices — not uploadable, would error on read.
		if !info.Mode().IsRegular() {
			return nil
		}
		holds[path.Dir(relSlash)] = true
		if o.onEntry != nil {
			o.onEntry(relSlash, info, "")
		}
		collected = append(collected, collectedFile{
			rel: relSlash, localAbs: p,
			size: info.Size(), mtime: info.ModTime(), mode: uint32(info.Mode().Perm()),
		})
		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	// Parallel hash: only files where size matches remote and --hash is on.
	// Without --hash, hashing is skipped entirely (mtime+size comparison only).
	toHash := make(map[string]string)
	for _, f := range collected {
		if r, ok := remoteSet[f.rel]; ok && !r.isDir && r.size == f.size && o.hash && r.hash != 0 {
			toHash[f.rel] = f.localAbs
		}
	}
	hashes := hashFilesParallel(toHash, o.parallel)

	// Classify each regular file: skip (unchanged) or queue for upload.
	for _, f := range collected {
		stats.scanned.Add(1)
		r, hadRemote := remoteSet[f.rel]
		delete(remoteSet, f.rel)

		if hadRemote && (r.isDir || r.linkTarget != "") {
			conflicts = append(conflicts, f.rel)
		} else if hadRemote && r.size == f.size {
			skip := false
			if o.hash && r.hash != 0 {
				skip = hashes[f.rel] == r.hash
				if skip {
					stats.hashSkipped.Add(1)
				}
			} else {
				skip = mtimeWithin(f.mtime, r.mtime, mtimeTolerance)
				if skip {
					stats.mtimeSkipped.Add(1)
				}
			}
			if skip && modeDiffers(f.mode, r.perm) {
				// Only the mode changed (chmod keeps size and mtime): set
				// it, rather than send the content again.
				if o.debug {
					fmt.Fprintf(os.Stderr, "  ? %s — mode differs: local=%o remote=%o\n", f.rel, f.mode, r.perm)
				}
				chmods = append(chmods, localDir{rel: f.rel, perm: f.mode})
				continue
			}
			if skip {
				stats.skipped.Add(1)
				continue
			}
		}

		if o.debug {
			fmt.Fprintf(os.Stderr, "  ? %s — %s\n", f.rel, uploadReason(hadRemote, r, f.size, f.mtime))
		}

		uploads = append(uploads, upload{
			rel:      f.rel,
			localAbs: f.localAbs,
			size:     f.size,
			mtime:    f.mtime,
			mode:     f.mode,
		})
	}

	// Folders the uploads won't make, or not with their mode.
	var makeDirs []localDir
	for _, d := range dirs {
		reason := ""
		switch {
		case d.onRemote:
			if d.perm != 0 && d.remotePerm != 0 && d.remotePerm != d.perm {
				reason = fmt.Sprintf("folder mode differs: local=%o remote=%o", d.perm, d.remotePerm)
			}
		case !holds[d.rel]:
			reason = "new empty folder"
		case d.perm != 0 && d.perm != defaultDirPerm:
			reason = fmt.Sprintf("new folder, mode %o", d.perm)
		}
		if reason != "" {
			if o.debug {
				fmt.Fprintf(os.Stderr, "  ? %s/ — %s\n", d.rel, reason)
			}
			makeDirs = append(makeDirs, d)
		}
	}

	// Delete phase: any path still in remoteSet wasn't in local → remove.
	// Pass the actual isDir from the remote meta so dirOnly exclude
	// patterns (e.g. "build/") shield remote directories correctly.
	extras := make([]string, 0, len(remoteSet))
	for rel, meta := range remoteSet {
		if excludes(rel, meta.isDir) || underAny(rel, conflicts) {
			continue
		}
		extras = append(extras, rel)
	}

	// Refuse to wipe the space from an empty local folder, as a mistyped
	// path or swapped arguments would.
	if stats.scanned.Load() == 0 && len(extras) > 0 {
		return stats, fmt.Errorf("%s is empty, so mirroring it would delete everything in %s:%s; check the path, or use --dry-run to see what would change", o.localRoot, o.spaceName, o.remoteDir)
	}

	// Conflicts go first, so what replaces them can be written. One that
	// couldn't be removed blocks what would replace it, and all below it:
	// a write would go through a symlink still there, landing outside the
	// mirrored folder. The next run tries again.
	if blocked := o.deleteExtras(ctx, conflicts, stats); len(blocked) > 0 {
		isBlocked := func(rel string) bool {
			for _, b := range blocked {
				if rel == b || strings.HasPrefix(rel, b+"/") {
					return true
				}
			}
			return false
		}
		uploads = slices.DeleteFunc(uploads, func(u upload) bool { return isBlocked(u.rel) })
		chmods = slices.DeleteFunc(chmods, func(d localDir) bool { return isBlocked(d.rel) })
		makeDirs = slices.DeleteFunc(makeDirs, func(d localDir) bool { return isBlocked(d.rel) })
	}

	// Upload phase: bounded parallel. The agent opens a folder made
	// read-only while it adds to it or removes from it.
	o.uploadInParallel(ctx, uploads, stats)

	o.setModes(ctx, chmods, stats)

	// Then the folders the uploads didn't make, and folder modes.
	o.makeDirs(ctx, makeDirs, stats)

	o.deleteExtras(ctx, extras, stats)

	return stats, nil
}

// runVerify does a read-only hash comparison between local and remote. No
// uploads, no deletes — just reports mismatches. The user runs this to
// confirm that a previous mirror cycle produced a byte-identical copy.
func (o *mirrorOptions) runVerify(ctx context.Context, remoteSet map[string]remoteMeta, stats *mirrorStats) (*mirrorStats, error) {
	var verified, mismatch, missingRemote int64
	excludes := compileExcludes(o.excludes)

	// Phase 1: walk and collect regular files. Symlinks are compared inline
	// (target, not content). Directories are removed from remoteSet.
	type verifyFile struct {
		rel       string
		abs       string
		size      int64
		remote    remoteMeta
		hadRemote bool
	}
	var files []verifyFile

	filepath.WalkDir(o.localRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == o.localRoot {
			return nil
		}
		if ctx.Err() != nil {
			return filepath.SkipAll
		}
		rel, _ := filepath.Rel(o.localRoot, p)
		relSlash := filepath.ToSlash(rel)
		if excludes(relSlash, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			delete(remoteSet, relSlash)
			return nil
		}
		// Symlink: compare target, not content hash.
		if d.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return nil
			}
			r, hadRemote := remoteSet[relSlash]
			delete(remoteSet, relSlash)
			if !hadRemote || r.isDir {
				missingRemote++
				fmt.Fprintf(os.Stderr, "  ✗ %s — missing on remote\n", relSlash)
				return nil
			}
			if r.linkTarget == target {
				verified++
				if o.verbose {
					fmt.Fprintf(os.Stderr, "  ✓ %s → %s\n", relSlash, target)
				}
			} else {
				mismatch++
				fmt.Fprintf(os.Stderr, "  ✗ %s — symlink target differs (local=%s remote=%s)\n", relSlash, target, r.linkTarget)
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}

		r, hadRemote := remoteSet[relSlash]
		delete(remoteSet, relSlash)

		if !hadRemote || r.isDir {
			missingRemote++
			fmt.Fprintf(os.Stderr, "  ✗ %s — missing on remote\n", relSlash)
			return nil
		}

		files = append(files, verifyFile{relSlash, p, info.Size(), r, hadRemote})
		return nil
	})

	if ctx.Err() != nil {
		return stats, ctx.Err()
	}

	// Phase 2: hash all collected files in parallel.
	toHash := make(map[string]string, len(files))
	for _, f := range files {
		toHash[f.rel] = f.abs
	}
	hashes := hashFilesParallel(toHash, o.parallel)

	// Phase 3: compare.
	for _, f := range files {
		localHash := hashes[f.rel]
		if localHash == f.remote.hash {
			verified++
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  ✓ %s\n", f.rel)
			}
		} else {
			mismatch++
			fmt.Fprintf(os.Stderr, "  ✗ %s — hash mismatch (local=%d remote=%d, size local=%d remote=%d)\n",
				f.rel, localHash, f.remote.hash, f.size, f.remote.size)
		}
	}

	// Anything left in remoteSet exists on the remote but not locally.
	// Only report FILES — remote-only directories are noise for content
	// verification (empty dirs don't carry content).
	missingLocal := int64(0)
	if ctx.Err() != nil {
		return stats, ctx.Err()
	}
	for rel, meta := range remoteSet {
		if meta.isDir {
			continue
		}
		if excludes(rel, false) {
			continue
		}
		missingLocal++
		fmt.Fprintf(os.Stderr, "  ✗ %s — missing on local\n", rel)
	}

	fmt.Fprintf(os.Stderr, "Verified: %d match, %d differ, %d missing on remote, %d missing on local\n",
		verified, mismatch, missingRemote, missingLocal)
	return stats, nil
}

// underAny reports whether rel lies below any of dirs.
func underAny(rel string, dirs []string) bool {
	for _, d := range dirs {
		if strings.HasPrefix(rel, d+"/") {
			return true
		}
	}
	return false
}

// mtimeWithin reports whether a and b are within tol of each other. Order
// doesn't matter; both directions are checked.
func mtimeWithin(a, b time.Time, tol time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// hashFilesParallel hashes the given files using workers goroutines.
// Returns a map of rel path → crc64 hash (0 on error). Used by both run()
// and runVerify() to parallelise the local-side hash computation.
func hashFilesParallel(files map[string]string, workers int) map[string]uint64 {
	if len(files) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	results := make(map[string]uint64, len(files))
	var mu sync.Mutex
	var wg sync.WaitGroup
	type job struct{ rel, abs string }
	jobCh := make(chan job, 64)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobCh {
				h := hashLocalFile(j.abs)
				mu.Lock()
				results[j.rel] = h
				mu.Unlock()
			}
		}()
	}
	for rel, abs := range files {
		jobCh <- job{rel, abs}
	}
	close(jobCh)
	wg.Wait()
	return results
}

// uploadReason returns a human-readable explanation for why a file is being
// queued for upload instead of skipped. Used by --debug.
func uploadReason(hadRemote bool, r remoteMeta, size int64, mtime time.Time) string {
	if !hadRemote {
		return "new (not on remote)"
	}
	if r.isDir {
		return "remote is a directory"
	}
	if r.linkTarget != "" {
		return "remote is a symlink"
	}
	if r.size != size {
		return fmt.Sprintf("size differs: local=%d remote=%d", size, r.size)
	}
	if r.hash != 0 {
		return fmt.Sprintf("hash mismatch (remote crc64=%d) — content differs despite same size", r.hash)
	}
	diff := mtime.Sub(r.mtime)
	if diff < 0 {
		diff = -diff
	}
	return fmt.Sprintf("mtime drift: local=%s remote=%s diff=%s (tol=%s)",
		mtime.UTC().Format(time.RFC3339Nano),
		r.mtime.UTC().Format(time.RFC3339Nano),
		diff.Round(time.Millisecond),
		mtimeTolerance)
}

// uploadInParallel runs N workers that drain the upload queue. Each worker
// handles one file at a time: if the file exceeds chunkSize it's sent in
// chunkSize-byte blocks (first overwrite, rest append, last carries mtime +
// perm); otherwise single-shot. One worker owns all chunks of its file — no
// cross-worker coordination needed. Failures don't abort the cycle.
func (o *mirrorOptions) uploadInParallel(ctx context.Context, uploads []upload, stats *mirrorStats) {
	jobs := make(chan upload)
	var wg sync.WaitGroup
	for i := 0; i < o.parallel; i++ {
		wg.Add(1)
		// One persistent buffer per worker — no allocations after startup.
		// OS demand-paging means only the pages actually written to are
		// backed by physical memory, so a worker handling small files
		// touches only a few KB of its chunkSize buffer.
		buf := make([]byte, chunkSize)
		go func(buf []byte) {
			defer wg.Done()
			for u := range jobs {
				if ctx.Err() != nil {
					return
				}
				if o.dryRun {
					if o.verbose {
						fmt.Fprintf(os.Stderr, "  ↑ %s (dry-run)\n", u.rel)
					}
					stats.uploaded.Add(1)
					continue
				}
				if err := o.uploadOne(ctx, u, buf); err != nil {
					o.failed(stats, u.rel, false, err)
					continue
				}
				stats.uploaded.Add(1)
				stats.bytes.Add(u.size)
				if o.verbose {
					fmt.Fprintf(os.Stderr, "  ↑ %s\n", u.rel)
				}
			}
		}(buf)
	}
	for _, u := range uploads {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- u:
		}
	}
	close(jobs)
	wg.Wait()
}

// bytesToString converts a byte slice to a string without copying. The
// returned string shares the backing array — safe as long as the slice is
// not mutated while the string is in use. Since WriteSpaceFileOpts
// serialises the string synchronously and the caller doesn't reuse the
// slice until after the call returns, this is always safe here. Saves one
// full-size allocation per uploaded file/chunk.
func bytesToString(b []byte) string {
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// uploadOne sends a single file to the space. Files larger than chunkSize
// are streamed in blocks using mode="overwrite" for the first block and
// mode="append" for the rest, so the server's 30s ReadTimeout never
// truncates a huge single-request body, to a hidden file that is then
// renamed into place with the source mtime and permission bits.
//
// buf is a caller-owned scratch buffer of at least chunkSize bytes. Using a
// persistent per-worker buffer avoids allocating a new buffer for every
// upload — the main source of GC pressure during bulk mirroring.
func (o *mirrorOptions) uploadOne(ctx context.Context, u upload, buf []byte) error {
	// Symlink: create link, don't write content.
	if u.symlinkTarget != "" {
		dest := path.Join(o.remoteDir, u.rel)
		return o.client.CreateSymlinkSpaceFile(ctx, o.spaceID, dest, u.symlinkTarget)
	}

	f, err := os.Open(u.localAbs)
	if err != nil {
		return fmt.Errorf("open %s: %w", u.localAbs, err)
	}
	defer f.Close()

	dest := path.Join(o.remoteDir, u.rel)

	// Files within chunkSize: single-shot read into buf, one request.
	if u.size <= int64(len(buf)) {
		n, err := io.ReadFull(f, buf)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return fmt.Errorf("read %s: %w", u.localAbs, err)
		}
		return o.client.WriteSpaceFileOpts(ctx, o.spaceID, dest, bytesToString(buf[:n]), "", u.mtime.UnixNano(), u.mode)
	}

	// Chunked: sent to a hidden file beside the destination, then renamed
	// over it, so the destination is never left part written.
	tmp := uploadTemp(dest)
	err = o.sendChunks(ctx, f, u, tmp, buf)
	if err == nil {
		err = o.client.RenameSpaceFile(ctx, o.spaceID, tmp, dest, u.mtime.UnixNano(), u.mode)
	}
	if err != nil && ctx.Err() == nil {
		// Don't leave the part sent behind; if the space can't be reached
		// to remove it, the next full mirror does.
		o.client.DeleteSpaceFile(ctx, o.spaceID, apiclient.DeleteFileRequest{Path: tmp})
	}
	return err
}

// uploadSuffix ends the name of the hidden file a large upload is sent to
// before it is renamed into place. One left by an upload cut short is
// removed by the next full mirror, as it is not here.
const uploadSuffix = ".knot-upload"

// uploadTemp is the hidden file beside dest that a large upload is sent to:
// ".<name>.<random>.knot-upload", the random part so it is never a file of
// the same name being mirrored, which the upload would overwrite.
func uploadTemp(dest string) string {
	var b [6]byte
	rand.Read(b[:])
	return path.Join(path.Dir(dest), "."+path.Base(dest)+"."+hex.EncodeToString(b[:])+uploadSuffix)
}

// sendChunks streams f to dest in chunkSize blocks, reusing buf for each:
// the first overwrites, the rest append.
func (o *mirrorOptions) sendChunks(ctx context.Context, f *os.File, u upload, dest string, buf []byte) error {
	sent := int64(0)
	first := true

	for sent < u.size {
		n, err := io.ReadFull(f, buf)

		mode := "append"
		if first {
			mode = "overwrite"
			first = false
		}

		if werr := o.client.WriteSpaceFileOpts(ctx, o.spaceID, dest, bytesToString(buf[:n]), mode, 0, 0); werr != nil {
			return werr
		}
		sent += int64(n)

		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			return fmt.Errorf("read %s: %w", u.localAbs, err)
		}
	}
	return nil
}

// localDir is a folder found here, and how the space has it.
type localDir struct {
	rel        string
	perm       uint32
	onRemote   bool
	remotePerm uint32
}

// defaultDirPerm is the mode the space gives the folders it makes for the
// files sent into them; a folder of another mode has it set.
const defaultDirPerm = 0755

// dirPerm is the folder mode to mirror: none from Windows, whose folders
// have no Unix mode.
func dirPerm(info os.FileInfo) uint32 {
	if runtime.GOOS == "windows" {
		return 0
	}
	return uint32(info.Mode().Perm())
}

// modeDiffers reports whether a file's permission bits here differ from the
// space's, which chmod changes without touching size or mtime. Windows has
// no Unix modes to compare, and an agent may give none.
func modeDiffers(local, remote uint32) bool {
	return runtime.GOOS != "windows" && remote != 0 && local != remote
}

// isUploadTemp reports whether rel is the hidden file a large upload is
// sent to before it is renamed into place.
func isUploadTemp(rel string) bool {
	name := path.Base(rel)
	return strings.HasPrefix(name, ".") && strings.HasSuffix(name, uploadSuffix)
}

// resolveRoot follows a local folder given as a symlink, which a walk would
// otherwise not enter.
func (o *mirrorOptions) resolveRoot() {
	if r, err := filepath.EvalSymlinks(o.localRoot); err == nil {
		o.localRoot = r
	}
}

// sortDirs orders folders parents first, or with deepestFirst the ones
// inside first, so a folder is never made read-only before the ones in it.
func sortDirs(dirs []localDir, deepestFirst bool) {
	sort.Slice(dirs, func(i, j int) bool { return (dirs[i].rel > dirs[j].rel) == deepestFirst })
}

// makeDirs makes folders on the space with their modes, those deepest
// first, so a folder is never made read-only before the ones in it.
func (o *mirrorOptions) makeDirs(ctx context.Context, dirs []localDir, stats *mirrorStats) {
	sortDirs(dirs, true)
	for _, d := range dirs {
		if ctx.Err() != nil {
			return
		}
		if o.dryRun {
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  ↑ %s/ (dry-run)\n", d.rel)
			}
			stats.uploaded.Add(1)
			continue
		}
		if err := o.makeDir(ctx, d.rel, d.perm); err != nil {
			o.failed(stats, d.rel, false, err)
			continue
		}
		stats.uploaded.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s/\n", d.rel)
		}
	}
}

// setModes sets the permission bits of files whose content the space
// already has.
func (o *mirrorOptions) setModes(ctx context.Context, files []localDir, stats *mirrorStats) {
	for _, f := range files {
		if ctx.Err() != nil {
			return
		}
		if !o.dryRun {
			if err := o.client.ChmodSpaceFile(ctx, o.spaceID, path.Join(o.remoteDir, f.rel), f.perm); err != nil {
				o.failed(stats, f.rel, false, err)
				continue
			}
		}
		stats.uploaded.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s (mode %o)\n", f.rel, f.perm)
		}
	}
}

// makeDir makes a folder on the space with its mode.
func (o *mirrorOptions) makeDir(ctx context.Context, rel string, perm uint32) error {
	return o.client.MakeSpaceDir(ctx, o.spaceID, path.Join(o.remoteDir, rel), perm)
}

// deleteExtras removes the given rel paths from the remote space. Each path
// is slash-relative under remoteDir; the destination path is computed and
// the delete is recursive (covers directories that may exist remotely).
func (o *mirrorOptions) deleteExtras(ctx context.Context, extras []string, stats *mirrorStats) (failed []string) {
	for _, rel := range extras {
		if ctx.Err() != nil {
			return failed
		}
		dest := path.Join(o.remoteDir, rel)
		if o.dryRun {
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  − %s (dry-run)\n", rel)
			}
			stats.deleted.Add(1)
			continue
		}
		// Recursive=true so a stale directory tree gets removed in one call.
		// Missing paths are already success at the agent, so no race risk.
		if _, err := o.client.DeleteSpaceFile(ctx, o.spaceID, apiclient.DeleteFileRequest{
			Path:      dest,
			Recursive: true,
		}); err != nil {
			o.failed(stats, rel, true, err)
			failed = append(failed, rel)
			continue
		}
		stats.deleted.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  − %s\n", rel)
		}
	}
	return failed
}

// fetchRemoteSet calls Find on the space and returns a map keyed by the path
// relative to remoteDir, carrying size/mtime/isDir. Mirror needs the metadata
// for the skip-unchanged comparison, so we set IncludeMetadata=true at the
// cost of a per-entry stat on the remote.
//
// IncludeHidden=true because mirror is supposed to mirror the source as-is,
// including .gitignore, .env, .task/, etc. Without it, every dotfile looks
// "new (not on remote)" forever — the local walker sees it, the agent's Find
// filters it out, no match → re-upload every run. Users who want to skip
// specific dotfiles should use --exclude.
func (o *mirrorOptions) fetchRemoteSet(ctx context.Context) (map[string]remoteMeta, error) {
	resp, err := o.client.Find(ctx, o.spaceID, apiclient.FindRequest{
		Path:            o.remoteDir,
		Recursive:       true,
		Type:            "any",
		IncludeHidden:   true,
		IncludeMetadata: true,
		IncludeHash:     o.hash,
		IncludeSymlinks: true,
	})
	if err != nil {
		return nil, err
	}
	set := make(map[string]remoteMeta, len(resp.Entries)+len(resp.Paths))
	// Mirror sets IncludeMetadata=true, so Entries is populated (not Paths).
	// Drain Paths too for defensiveness in case a test or future caller
	// flips the flag — entries without metadata are recorded with zero
	// size/mtime, which forces an upload on the next comparison.
	for _, p := range resp.Paths {
		rel := relativiseRemote(o.remoteDir, p)
		if rel == "" {
			continue
		}
		set[rel] = remoteMeta{}
	}
	for _, e := range resp.Entries {
		rel := relativiseRemote(o.remoteDir, e.Path)
		if rel == "" {
			continue
		}
		set[rel] = remoteMeta{
			size:       e.Size,
			mtime:      time.Unix(0, int64(e.Mtime*1e9)),
			isDir:      e.IsDir,
			hash:       e.Hash,
			linkTarget: e.LinkTarget,
			perm:       uint32(e.FilePerm),
		}
	}
	return set, nil
}

// compileExcludes returns a function that reports whether a slash-relative
// path should be skipped; see excludes.Compile.
func compileExcludes(patterns []string) func(rel string, isDir bool) bool {
	return excludes.Compile(patterns)
}

// relativiseRemote returns the part of sub that lies under base, slash-
// separated. Both inputs are slash-style. Empty if sub isn't under base.
func relativiseRemote(base, sub string) string {
	cb := path.Clean(base)
	cs := path.Clean(sub)
	if cb == "" || cb == "." {
		return strings.TrimPrefix(cs, "/")
	}
	if cb == cs {
		return ""
	}
	prefix := cb + "/"
	if !strings.HasPrefix(cs, prefix) {
		return ""
	}
	return strings.TrimPrefix(cs, prefix)
}

// Compile-time check: *apiclient.ApiClient satisfies mirrorClient.
var _ mirrorClient = (*apiclient.ApiClient)(nil)
