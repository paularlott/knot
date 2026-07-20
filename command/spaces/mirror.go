package command_spaces

import (
	"context"
	"fmt"
	"hash/crc64"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

// MirrorCmd mirrors a local directory tree to a space. Source is local,
// destination is <space>:<path>. Uploads every file in the tree (preserving
// each file's mtime and permission bits) and deletes any remote file that
// doesn't exist locally — the destination ends up as a mirror of the source.
//
//   knot space mirror ./src myspace:/var/www/html
//
// For one-way upload without deletes, use knot space copy per-file or write
// your own loop. For continuous two-way sync, mutagen against the space's SSH
// endpoint is the recommendation; mirror is designed for one-shot publishing
// of a tree.
var MirrorCmd = &cli.Command{
	Name:        "mirror",
	Usage:       "Mirror a local directory to a space",
	Description: "Upload <local folder> to <space>:<path>, then delete remote files that don't exist locally. The destination ends up as a mirror of the source. For one-shot upload without deletes, use `knot space copy` per-file.",
	Flags: []cli.Flag{
		&cli.StringSliceFlag{
			Name:    "exclude",
			Aliases: []string{"x"},
			Usage:   "Glob patterns to skip (e.g. node_modules, *.log). Repeatable.",
		},
		&cli.IntFlag{
			Name:         "parallel",
			Usage:        "Concurrent upload workers (default 8)",
			DefaultValue: 8,
		},
		&cli.BoolFlag{
			Name:  "dry-run",
			Usage: "List what would be uploaded/deleted without performing any I/O",
		},
		&cli.BoolFlag{
			Name:  "debug",
			Usage: "Log why each file was uploaded (new / size differs / mtime drift) — for diagnosing idempotence issues",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "Print every upload and delete as it happens (default: summary only)",
		},
		&cli.BoolFlag{
			Name:  "verify",
			Usage: "Compare local and remote hashes without uploading or deleting; report any mismatches",
		},
		&cli.BoolFlag{
			Name:  "hash",
			Usage: "Use crc64 hash comparison instead of mtime+size (slower but definitive — catches content drift that mtime misses)",
		},
	},
	Arguments: []cli.Argument{
		&cli.StringArg{
			Name:     "local",
			Required: true,
			Usage:    "Local folder to mirror",
		},
		&cli.StringArg{
			Name:     "remote",
			Required: true,
			Usage:    "Destination in the form 'space:path'",
		},
	},
	MaxArgs: cli.NoArgs,
	Run: func(ctx context.Context, cmd *cli.Command) error {
		local := cmd.GetStringArg("local")
		remote := cmd.GetStringArg("remote")

		colon := strings.Index(remote, ":")
		if colon <= 1 {
			return fmt.Errorf("remote must be in the form 'space:path'")
		}
		spaceName := remote[:colon]
		remoteDir := remote[colon+1:]
		if remoteDir == "" {
			return fmt.Errorf("remote space path cannot be empty after '%s:'", spaceName)
		}

		info, err := os.Stat(local)
		if err != nil {
			return fmt.Errorf("stat local: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("local path must be a directory (got %q)", local)
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
			debug: cmd.GetBool("debug"),
			// --debug implies --verbose: skip-reason lines are meaningless
			// without the upload lines for context.
			verbose: cmd.GetBool("verbose") || cmd.GetBool("debug"),
			verify:  cmd.GetBool("verify"),
			hash:    cmd.GetBool("hash") || cmd.GetBool("verify"), // verify always uses hash
		}
		if opts.parallel < 1 {
			opts.parallel = 1
		}

		if opts.verify {
			fmt.Fprintf(os.Stderr, "Verifying %s ↔ %s:%s\n", local, spaceName, remoteDir)
		} else {
			fmt.Fprintf(os.Stderr, "Mirroring %s → %s:%s\n", local, spaceName, remoteDir)
		}
		start := time.Now()
		stats, err := opts.run(ctx)
		if err != nil {
			return err
		}
		if !opts.verify {
			fmt.Fprintf(os.Stderr, "Done in %s\n", stats.String(time.Since(start)))
		}
		return nil
	},
}

// mirrorStats tracks one cycle's counts. All atomic — workers mutate
// concurrently. Honest counts even when individual ops fail (the cycle
// keeps going).
type mirrorStats struct {
	scanned      atomic.Int64
	uploaded     atomic.Int64
	skipped      atomic.Int64
	deleted      atomic.Int64
	failed       atomic.Int64
	bytes        atomic.Int64
	hashSkipped  atomic.Int64 // skipped via definitive hash comparison
	mtimeSkipped atomic.Int64 // skipped via mtime fallback (agent too old for hashing)
}

func (s *mirrorStats) String(duration time.Duration) string {
	skipped := s.skipped.Load()
	detail := ""
	if skipped > 0 {
		h := s.hashSkipped.Load()
		m := s.mtimeSkipped.Load()
		if h > 0 || m > 0 {
			detail = fmt.Sprintf(" (%d hash, %d mtime)", h, m)
		}
	}
	return fmt.Sprintf("%d scanned, %d uploaded, %d skipped%s, %d deleted, %d failed (%s)",
		s.scanned.Load(), s.uploaded.Load(), skipped, detail,
		s.deleted.Load(), s.failed.Load(), duration.Round(time.Millisecond))
}

// chunkSize is the threshold for chunked uploads. Files larger than this are
// uploaded in chunkSize-byte blocks: the first block uses mode="overwrite",
// subsequent blocks use mode="append". The last block carries the mtime +
// permission bits for applySyncMetadata on the agent side.
//
// A var (not const) so tests can shrink it to exercise the chunked path
// without writing hundreds of megabytes.
var chunkSize = int64(64 * 1024 * 1024) // 64 MB — power of 2, fits under common server buffer limits

// mtimeTolerance is the fallback window used only when the agent didn't
// return a hash (Hash=0). When hashes are available, comparison is
// definitive and this tolerance is irrelevant.
const mtimeTolerance = 1 * time.Second

// crc64ISO is the table used for local file hashing. Must match the agent's
// hashFile in extlibs/find.go (crc64.ISO).
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
}

// mirrorClient is the subset of *apiclient.ApiClient that mirror needs.
// *apiclient.ApiClient satisfies it implicitly.
type mirrorClient interface {
	Find(ctx context.Context, spaceID string, req apiclient.FindRequest) (*apiclient.FindResponse, error)
	WriteSpaceFileOpts(ctx context.Context, spaceID, filePath, content, mode string, mtimeNs int64, filePerm uint32) error
	DeleteSpaceFile(ctx context.Context, spaceID string, req apiclient.DeleteFileRequest) (*apiclient.DeleteFileResponse, error)
	CreateSymlinkSpaceFile(ctx context.Context, spaceID, filePath, target string) error
}

// remoteMeta is the subset of FindEntry that mirror compares against local
// files to decide whether an upload can be skipped.
type remoteMeta struct {
	size       int64
	mtime      time.Time
	isDir      bool
	hash       uint64
	linkTarget string // symlink target; empty for regular files
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

	// Fetch remote set WITH metadata + hash. The per-entry stat and hash on
	// the remote is cheap compared to re-uploading unchanged files.
	remoteSet, err := o.fetchRemoteSet(ctx)
	if err != nil {
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

	// Walk local, producing upload jobs. Files that match a remote entry
	// (size + hash) are skipped — counted but not uploaded.
	var uploads []upload

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
			// Directories are created implicitly by file uploads (the agent
			// MkdirAll's the parent). Remove from the remote set so the
			// delete phase doesn't try to remove them as extras.
			delete(remoteSet, relSlash)
			return nil
		}
		// Symlink: read target, compare against remote, queue or skip.
		if d.Type()&os.ModeSymlink != 0 {
			stats.scanned.Add(1)
			target, err := os.Readlink(p)
			if err != nil {
				return nil
			}
			r, hadRemote := remoteSet[relSlash]
			delete(remoteSet, relSlash)
			if hadRemote && r.linkTarget == target {
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
		stats.scanned.Add(1)

		// Skip-if-unchanged: when --hash is set, compare crc64 (definitive).
		// Default: mtime + size heuristic (fast, no file reads).
		r, hadRemote := remoteSet[relSlash]
		if hadRemote && !r.isDir && r.size == info.Size() {
			skip := false
			if o.hash && r.hash != 0 {
				skip = hashLocalFile(p) == r.hash
				if skip {
					stats.hashSkipped.Add(1)
				}
			} else {
				skip = mtimeWithin(info.ModTime(), r.mtime, mtimeTolerance)
				if skip {
					stats.mtimeSkipped.Add(1)
				}
			}
			if skip {
				stats.skipped.Add(1)
				delete(remoteSet, relSlash)
				return nil
			}
		}

		// Capture the reason before we delete from the set; used by --debug.
		var reason string
		if o.debug {
			reason = uploadReason(hadRemote, r, info)
		}

		// Drop from remote set so the delete phase sees only true extras.
		delete(remoteSet, relSlash)

		if o.debug {
			fmt.Fprintf(os.Stderr, "  ? %s — %s\n", relSlash, reason)
		}

		uploads = append(uploads, upload{
			rel:      relSlash,
			localAbs: p,
			size:     info.Size(),
			mtime:    info.ModTime(),
			mode:     uint32(info.Mode().Perm()),
		})
		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	// Upload phase: bounded parallel.
	o.uploadInParallel(ctx, uploads, stats)

	// Delete phase: any path still in remoteSet wasn't in local → remove.
	// Pass the actual isDir from the remote meta so dirOnly exclude
	// patterns (e.g. "build/") shield remote directories correctly.
	extras := make([]string, 0, len(remoteSet))
	for rel, meta := range remoteSet {
		if excludes(rel, meta.isDir) {
			continue
		}
		extras = append(extras, rel)
	}
	o.deleteExtras(ctx, extras, stats)

	return stats, nil
}

// runVerify does a read-only hash comparison between local and remote. No
// uploads, no deletes — just reports mismatches. The user runs this to
// confirm that a previous mirror cycle produced a byte-identical copy.
func (o *mirrorOptions) runVerify(ctx context.Context, remoteSet map[string]remoteMeta, stats *mirrorStats) (*mirrorStats, error) {
	var verified, mismatch, missingRemote int64
	excludes := compileExcludes(o.excludes)

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
		// Directories: remove from remoteSet so they don't show as "missing
		// on local". Mirror only verifies file content, not empty dirs.
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

		localHash := hashLocalFile(p)
		if localHash == r.hash {
			verified++
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  ✓ %s\n", relSlash)
			}
		} else {
			mismatch++
			fmt.Fprintf(os.Stderr, "  ✗ %s — hash mismatch (local=%d remote=%d, size local=%d remote=%d)\n",
				relSlash, localHash, r.hash, info.Size(), r.size)
		}
		return nil
	})

	// Anything left in remoteSet exists on the remote but not locally.
	// Only report FILES — remote-only directories are noise for content
	// verification (empty dirs don't carry content and mirror doesn't
	// create or delete bare directories).
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

// mtimeWithin reports whether a and b are within tol of each other. Order
// doesn't matter; both directions are checked.
func mtimeWithin(a, b time.Time, tol time.Duration) bool {
	d := a.Sub(b)
	if d < 0 {
		d = -d
	}
	return d <= tol
}

// uploadReason returns a human-readable explanation for why a file is being
// queued for upload instead of skipped. Used by --debug to diagnose
// idempotence issues (e.g. mtimes that keep drifting).
func uploadReason(hadRemote bool, r remoteMeta, info os.FileInfo) string {
	if !hadRemote {
		return "new (not on remote)"
	}
	if r.isDir {
		return "remote is a directory"
	}
	if r.size != info.Size() {
		return fmt.Sprintf("size differs: local=%d remote=%d", info.Size(), r.size)
	}
	if r.hash != 0 {
		return fmt.Sprintf("hash mismatch (remote crc64=%d) — content differs despite same size", r.hash)
	}
	diff := info.ModTime().Sub(r.mtime)
	if diff < 0 {
		diff = -diff
	}
	return fmt.Sprintf("mtime drift: local=%s remote=%s diff=%s (tol=%s)",
		info.ModTime().UTC().Format(time.RFC3339Nano),
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
		go func() {
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
				if err := o.uploadOne(ctx, u); err != nil {
					stats.failed.Add(1)
					fmt.Fprintf(os.Stderr, "  ! %s: %v\n", u.rel, err)
					continue
				}
				stats.uploaded.Add(1)
				stats.bytes.Add(u.size)
				if o.verbose {
					fmt.Fprintf(os.Stderr, "  ↑ %s\n", u.rel)
				}
			}
		}()
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

// uploadOne sends a single file to the space. Files larger than chunkSize
// are streamed in blocks using mode="overwrite" for the first block and
// mode="append" for the rest, so the server's 30s ReadTimeout never
// truncates a huge single-request body. The last block carries the source
// mtime and permission bits for applySyncMetadata on the agent side.
func (o *mirrorOptions) uploadOne(ctx context.Context, u upload) error {
	// Symlink: create link, don't write content.
	if u.symlinkTarget != "" {
		dest := path.Join(o.remoteDir, u.rel)
		return o.client.CreateSymlinkSpaceFile(ctx, o.spaceID, dest, u.symlinkTarget)
	}

	if u.size <= chunkSize {
		// Single-shot: read once, send with metadata.
		content, err := os.ReadFile(u.localAbs)
		if err != nil {
			return fmt.Errorf("read %s: %w", u.localAbs, err)
		}
		dest := path.Join(o.remoteDir, u.rel)
		return o.client.WriteSpaceFileOpts(ctx, o.spaceID, dest, string(content), "", u.mtime.UnixNano(), u.mode)
	}

	// Chunked: stream chunkSize blocks. One worker, sequential blocks.
	f, err := os.Open(u.localAbs)
	if err != nil {
		return fmt.Errorf("open %s: %w", u.localAbs, err)
	}
	defer f.Close()

	dest := path.Join(o.remoteDir, u.rel)
	buf := make([]byte, int(chunkSize))
	sent := int64(0)
	first := true

	for sent < u.size {
		n, err := io.ReadFull(f, buf)
		isLast := sent+int64(n) >= u.size

		mode := "append"
		var mtimeNs int64
		var perm uint32
		if first {
			mode = "overwrite"
			first = false
		}
		if isLast {
			mtimeNs = u.mtime.UnixNano()
			perm = u.mode
		}

		if werr := o.client.WriteSpaceFileOpts(ctx, o.spaceID, dest, string(buf[:n]), mode, mtimeNs, perm); werr != nil {
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

// deleteExtras removes the given rel paths from the remote space. Each path
// is slash-relative under remoteDir; the destination path is computed and
// the delete is recursive (covers directories that may exist remotely).
func (o *mirrorOptions) deleteExtras(ctx context.Context, extras []string, stats *mirrorStats) {
	for _, rel := range extras {
		if ctx.Err() != nil {
			return
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
			stats.failed.Add(1)
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			continue
		}
		stats.deleted.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  − %s\n", rel)
		}
	}
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
		}
	}
	return set, nil
}

// compileExcludes returns a function that reports whether a slash-relative
// path should be skipped. Each pattern is matched against the basename and
// the full relative path; ancestor matches make excludes transitive (so
// "node_modules" excludes everything under it).
func compileExcludes(patterns []string) func(rel string, isDir bool) bool {
	type compiled struct {
		glob    string
		dirOnly bool
	}
	var cs []compiled
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		dirOnly := false
		if strings.HasSuffix(p, "/") {
			dirOnly = true
			p = strings.TrimSuffix(p, "/")
		}
		p = path.Clean(p)
		if p == "." || p == "" {
			continue
		}
		cs = append(cs, compiled{glob: p, dirOnly: dirOnly})
	}
	return func(rel string, isDir bool) bool {
		base := rel
		if i := strings.LastIndexByte(rel, '/'); i >= 0 {
			base = rel[i+1:]
		}
		// Direct match.
		for _, c := range cs {
			if c.dirOnly && !isDir {
				continue
			}
			if ok, _ := filepath.Match(c.glob, rel); ok {
				return true
			}
			if base != rel {
				if ok, _ := filepath.Match(c.glob, base); ok {
					return true
				}
			}
		}
		// Ancestor match — makes excludes transitive.
		for _, c := range cs {
			if ancestorMatches(rel, c.glob) {
				return true
			}
		}
		return false
	}
}

// ancestorMatches reports whether any parent directory of rel matches glob.
// "a/b/c.txt" checks "a/b" then "a".
func ancestorMatches(rel, glob string) bool {
	p := rel
	for {
		i := strings.LastIndexByte(p, '/')
		if i < 0 {
			return false
		}
		parent := p[:i]
		parentBase := parent
		if j := strings.LastIndexByte(parent, '/'); j >= 0 {
			parentBase = parent[j+1:]
		}
		if ok, _ := filepath.Match(glob, parent); ok {
			return true
		}
		if parentBase != parent {
			if ok, _ := filepath.Match(glob, parentBase); ok {
				return true
			}
		}
		p = parent
	}
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
