package command_spaces

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util/dirwatch"
)

// debounceInterval is how long the watcher waits after the last file system
// event of a burst before sending. Editors that save atomically (write temp
// → rename) produce 3-5 events in <50ms; this window collapses them into a
// single upload.
const debounceInterval = 300 * time.Millisecond

const (
	// maxSettle bounds how long a burst of changes that keeps going, such
	// as a file being written, holds up sending.
	maxSettle = 2 * time.Second
	// stopGrace lets the watcher report a change made just before Ctrl+C.
	stopGrace = 150 * time.Millisecond
)

const (
	// rescanEvery is how often the whole folder is mirrored again while
	// some folders could not be watched, to find the changes in them.
	rescanEvery = 5 * time.Minute
	// retryMin and retryMax bound the wait before a change that could not be
	// sent, the space restarting for example, is tried again, doubling each
	// time.
	retryMin = 2 * time.Second
	retryMax = time.Minute
)

// watchDefaultExcludes are patterns always excluded in watch mode. Vim swap
// files (.*.swp) appear and disappear rapidly during edits — uploading them
// is pure noise.
var watchDefaultExcludes = []string{".*.swp", ".*.swo", ".*.swn"}

// runWatch performs the initial mirror, then watches the local tree for
// incremental changes and syncs them to the space. One-way: local → remote.
// Blocks until Ctrl+C / SIGTERM.
//
// The tree is watched with FSEvents on macOS (one stream, however large the
// tree), else fsnotify. Watching starts before the initial mirror, so a change
// made while it runs is not missed. Changes that cannot be sent are tried
// again with backoff; when events are lost, or folders cannot be watched, the
// whole folder is mirrored again.
func (o *mirrorOptions) runWatch(ctx context.Context) error {
	// Add watch-specific excludes (vim swap files, etc.) so both the initial
	// mirror and the live watcher skip them.
	o.excludes = append(o.excludes, watchDefaultExcludes...)

	// Cancel-on-signal so the event loop exits cleanly on Ctrl+C. The parent
	// ctx comes from the CLI and is NOT signal-aware (the framework doesn't
	// install a handler).
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	w, err := newMirrorWatcher(o)
	if err != nil {
		return err
	}
	defer w.close()

	start := time.Now()
	stats, err := w.mirror(watchCtx)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Done: %s\n", stats.String(time.Since(start)))
	fmt.Fprintf(os.Stderr, "Watching for changes... (Ctrl+C to stop)\n")

	return w.loop(ctx, watchCtx, stop)
}

// entryKind is what is at a path.
type entryKind uint8

const (
	kindNone entryKind = iota
	kindFile
	kindDir
	kindLink
	// kindOther: something the space has that a delete failed to remove,
	// of a kind not known here, so whatever replaces it removes it first.
	kindOther
)

// sentEntry is what the space was last given at a path: enough to tell
// whether the local path has changed since, and whether its kind has.
type sentEntry struct {
	kind   entryKind
	size   int64
	mtime  int64 // unix nanoseconds
	mode   uint32
	target string // symlink target
}

func entryOf(info os.FileInfo, target string) sentEntry {
	switch {
	case info.IsDir():
		return sentEntry{kind: kindDir, mode: dirPerm(info)}
	case info.Mode()&os.ModeSymlink != 0:
		return sentEntry{kind: kindLink, target: target}
	case info.Mode().IsRegular():
		return sentEntry{kind: kindFile, size: info.Size(), mtime: info.ModTime().UnixNano(), mode: uint32(info.Mode().Perm())}
	}
	return sentEntry{}
}

// mirrorWatcher turns file system events into uploads and deletes. All of
// its state is used from the event loop's goroutine only.
type mirrorWatcher struct {
	opts *mirrorOptions
	// root is localRoot as the watcher reports it: absolute, with symlinks
	// resolved (FSEvents reports /private/var for /var, for example).
	root      string
	excludes  func(rel string, isDir bool) bool
	dw        dirwatch.Watcher
	pending   map[string]bool      // slash-relative paths to look at
	known     map[string]sentEntry // what the space was last given at each path
	failed    map[string]bool      // paths whose last send failed, to try again
	reported  map[string]string    // the last error shown for a path, not repeated
	reportMu  sync.Mutex           // guards failed and reported while a full mirror's uploads run
	buf       []byte               // scratch buffer for uploads, avoids per-file allocation
	unwatched atomic.Bool          // some folders could not be watched
	stopping  bool                 // the last pass, as the watch stops: nothing is retried
}

func newMirrorWatcher(o *mirrorOptions) (*mirrorWatcher, error) {
	root, err := filepath.Abs(o.localRoot)
	if err != nil {
		return nil, err
	}
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	w := &mirrorWatcher{
		opts:     o,
		root:     root,
		excludes: compileExcludes(o.excludes),
		pending:  make(map[string]bool),
		known:    make(map[string]sentEntry),
		failed:   make(map[string]bool),
		reported: make(map[string]string),
		buf:      make([]byte, chunkSize),
	}
	w.dw, err = dirwatch.New(dirwatch.Options{
		Root:     root,
		Excluded: w.excludes,
		Label:    "knot.space.mirror",
		Warn: func(msg string, err error) {
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", msg, err)
		},
		Unwatched: func(failed, total int, firstErr error) {
			if !w.unwatched.Swap(true) {
				fmt.Fprintf(os.Stderr, "  ! cannot watch %d of %d folders, so changes in them are found by mirroring again every %s: %s\n",
					failed, total, rescanEvery, dirwatch.LimitHint(firstErr, total))
			}
		},
	})
	if err != nil {
		return nil, fmt.Errorf("watch %s: %w", o.localRoot, err)
	}
	w.dw.Add(root)
	return w, nil
}

func (w *mirrorWatcher) close() {
	if w.dw != nil {
		w.dw.Close()
	}
}

// loop handles events until watchCtx ends, then sends what changed and was
// not yet sent. ctx outlives watchCtx for that last flush; stop restores the
// default signal handling so a second Ctrl+C ends it at once.
//
// Live changes are sent once they settle: debounceInterval after the last
// event, or maxSettle after the first of a burst that keeps going. Paths
// that failed, and full mirrors, are tried again on their own timer, so one
// that keeps failing never holds up live changes.
func (w *mirrorWatcher) loop(ctx, watchCtx context.Context, stop func()) error {
	events := w.dw.Events()
	var settle <-chan time.Time // live changes, once they settle
	var burst time.Time         // when the unsent burst of events began
	var again <-chan time.Time  // a full mirror, or a retry of what failed
	var retryIn time.Duration
	retry := func() <-chan time.Time {
		retryIn = min(max(2*retryIn, retryMin), retryMax)
		return time.After(retryIn)
	}
	rescan := false
	if len(w.failed) > 0 {
		again = retry()
	}
	lastRescan := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-watchCtx.Done():
			stop()
			// Let the watcher report what changed in the last moment.
			time.Sleep(stopGrace)
			w.drain(events)
			for rel := range w.failed {
				w.pending[rel] = true
			}
			if len(w.pending) > 0 {
				fmt.Fprintf(os.Stderr, "\nSending the last changes... (Ctrl+C again to stop now)\n")
				w.stopping = true
				w.flush(ctx)
			}
			fmt.Fprintf(os.Stderr, "\nStopped watching.\n")
			return nil

		case ev, ok := <-events:
			if !ok {
				return nil
			}
			if w.onEvent(ev) {
				rescan = true
				if again == nil {
					again = time.After(debounceInterval)
				}
				continue
			}
			now := time.Now()
			if burst.IsZero() {
				burst = now
			}
			settle = time.After(max(0, min(debounceInterval, maxSettle-now.Sub(burst))))

		case <-tick.C:
			if w.dw.TakeLost() {
				if w.opts.verbose {
					fmt.Fprintf(os.Stderr, "  file system events were lost: mirroring again\n")
				}
				rescan = true
			}
			if w.unwatched.Load() && time.Since(lastRescan) >= rescanEvery {
				rescan = true
			}
			if rescan && again == nil {
				again = time.After(0)
			}

		case <-settle:
			settle, burst = nil, time.Time{}
			if !w.flush(watchCtx) && watchCtx.Err() == nil && again == nil {
				again = retry()
			}

		case <-again:
			again = nil
			if rescan {
				rescan = false
				lastRescan = time.Now()
				if !w.mirrorAgain(watchCtx) {
					if watchCtx.Err() == nil {
						rescan = true
						again = retry()
					}
					continue
				}
				// The full mirror covered what was pending; what it
				// couldn't send is tried again like any failed path.
				clear(w.pending)
				if len(w.failed) > 0 {
					again = retry()
				} else {
					retryIn = 0
				}
				continue
			}
			if !w.flush(watchCtx) && watchCtx.Err() == nil {
				again = retry()
				continue
			}
			retryIn = 0
		}
	}
}

// drain takes in the events already reported, without waiting for more.
func (w *mirrorWatcher) drain(events <-chan dirwatch.Event) {
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			w.onEvent(ev)
		default:
			return
		}
	}
}

// mirrorAgain mirrors the whole folder, finding what the events missed. It
// reports false only when the mirror could not run at all; what it could
// not send is left in failed, to be tried again.
func (w *mirrorWatcher) mirrorAgain(ctx context.Context) bool {
	start := time.Now()
	stats, err := w.mirror(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.report("", err)
		}
		return false
	}
	delete(w.reported, "")
	if stats.uploaded.Load()+stats.deleted.Load()+stats.failed.Load() > 0 {
		fmt.Fprintf(os.Stderr, "Mirrored again: %s\n", stats.String(time.Since(start)))
	}
	return true
}

// mirror runs a full mirror and records what it gave the space: what its
// walk saw, not what is here once it ends, so a change made while it ran is
// still sent when its event is handled. A path it could not send is left
// out, and noted to be tried again; its error is shown once.
func (w *mirrorWatcher) mirror(ctx context.Context) (*mirrorStats, error) {
	seen := make(map[string]sentEntry)
	var seenMu sync.Mutex
	w.opts.onEntry = func(rel string, info os.FileInfo, target string) {
		if e := entryOf(info, target); e.kind != kindNone {
			seenMu.Lock()
			seen[rel] = e
			seenMu.Unlock()
		}
	}
	w.opts.onFail = func(rel string, deleting bool, err error) {
		seenMu.Lock()
		if deleting {
			// Still in the space, as something else: known, so the retry
			// deletes it before anything is written there.
			seen[rel] = sentEntry{kind: kindOther}
			// What the walk saw below it was never sent.
			for p := range seen {
				if strings.HasPrefix(p, rel+"/") {
					delete(seen, p)
				}
			}
		} else {
			delete(seen, rel)
		}
		seenMu.Unlock()
		w.reportMu.Lock()
		w.fail(rel, err)
		w.reportMu.Unlock()
	}
	defer func() { w.opts.onEntry, w.opts.onFail = nil, nil }()

	stats, err := w.opts.run(ctx)
	if err != nil {
		return stats, err
	}
	w.known = seen
	return stats, nil
}

// walk calls fn for every path below dir that is not excluded.
func (w *mirrorWatcher) walk(dir string, fn func(rel, abs string, info os.FileInfo)) {
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == dir {
			return nil
		}
		rel := w.rel(p)
		if rel == "" {
			return nil
		}
		if w.excludes(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, err := d.Info(); err == nil {
			fn(rel, p, info)
		}
		return nil
	})
}

// rel converts an absolute local path to a slash-relative path under root.
// Returns "" for the root itself or paths outside the tree.
func (w *mirrorWatcher) rel(absPath string) string {
	rel, err := filepath.Rel(w.root, absPath)
	if err != nil {
		return ""
	}
	s := filepath.ToSlash(rel)
	if s == "." || s == ".." || strings.HasPrefix(s, "../") {
		return ""
	}
	return s
}

// onEvent notes the path an event touched, to look at once the burst
// settles. It reports true when events were lost and the whole folder must
// be mirrored again.
func (w *mirrorWatcher) onEvent(ev dirwatch.Event) bool {
	if ev.All {
		return true
	}
	rel := w.rel(ev.Path)
	if rel == "" {
		// The root itself: events below it were lost, or it changed.
		return ev.Below
	}
	// For a path that is gone, stat fails and isDir is unknown. That's
	// fine: ancestor exclude checks don't depend on isDir, and dir-only
	// patterns only shield new dirs (which a removal can't be).
	isDir := false
	if info, err := os.Lstat(ev.Path); err == nil {
		isDir = info.IsDir()
	}
	if w.excludes(rel, isDir) {
		return false
	}
	// A folder is looked at whole whatever the event (see syncPath), so
	// events lost below one need nothing more.
	w.pending[rel] = true
	return false
}

// flush sends what is pending, parents before what they hold, and reports
// whether all of it was sent. Paths that failed are kept, to be tried again.
func (w *mirrorWatcher) flush(ctx context.Context) bool {
	for rel := range w.failed {
		w.pending[rel] = true
	}
	clear(w.failed)
	if len(w.pending) == 0 {
		return true
	}
	rels := make([]string, 0, len(w.pending))
	for rel := range w.pending {
		rels = append(rels, rel)
	}
	clear(w.pending)
	sort.Strings(rels)

	// A folder looked at whole, or deleted, covers what it holds.
	var covered []string
	isCovered := func(rel string) bool {
		for _, c := range covered {
			if strings.HasPrefix(rel, c+"/") {
				return true
			}
		}
		return false
	}
	for i, rel := range rels {
		if ctx.Err() != nil {
			for _, r := range rels[i:] {
				w.failed[r] = true
			}
			break
		}
		if isCovered(rel) {
			continue
		}
		if w.syncPath(ctx, rel) {
			covered = append(covered, rel)
		}
	}
	return len(w.failed) == 0
}

// syncPath brings a single local path into sync with the space. A path that
// is gone is deleted from the space; a file or symlink is uploaded; a folder
// is compared, all of it, with what the space was last given, sending what
// changed and deleting what went. A path whose kind changed (a file now a
// folder, a symlink now a file) is removed from the space first, so a file is
// never written through an old symlink or onto a folder. It reports whether
// it dealt with everything below rel.
func (w *mirrorWatcher) syncPath(ctx context.Context, rel string) bool {
	localAbs := filepath.Join(w.root, filepath.FromSlash(rel))

	info, err := os.Lstat(localAbs)
	if err != nil {
		// Path gone → delete from remote (recursive covers directories),
		// unless nothing was ever sent there, such as an editor's temp file.
		w.dw.Remove(localAbs)
		if w.knowsAt(rel) && w.deleteRemote(ctx, rel) {
			w.forget(rel)
		}
		return true
	}

	// Re-check excludes — the path type may have changed since the event
	// was queued (e.g. a file replaced by a directory).
	if w.excludes(rel, info.IsDir()) {
		return true
	}
	if info.IsDir() {
		// Watch it (a new folder), then send what changed inside.
		w.dw.Add(localAbs)
		w.syncTree(ctx, localAbs, rel, info)
		return true
	}
	w.syncEntry(ctx, localAbs, rel, info, true)
	return false
}

// syncTree brings a folder and all below it into line with the space.
func (w *mirrorWatcher) syncTree(ctx context.Context, localAbs, rel string, info os.FileInfo) {
	if !w.syncEntry(ctx, localAbs, rel, info, false) {
		return
	}
	seen := map[string]bool{rel: true}
	w.walk(localAbs, func(r, abs string, info os.FileInfo) {
		if ctx.Err() != nil {
			return
		}
		seen[r] = true
		w.syncEntry(ctx, abs, r, info, false)
	})
	if ctx.Err() != nil {
		return
	}
	// What was sent below the folder and is here no more.
	prefix := rel + "/"
	var gone []string
	for p := range w.known {
		if strings.HasPrefix(p, prefix) && !seen[p] {
			gone = append(gone, p)
		}
	}
	sort.Strings(gone)
	var removed []string
	for _, p := range gone {
		inRemoved := false
		for _, d := range removed {
			if strings.HasPrefix(p, d+"/") {
				inRemoved = true
				break
			}
		}
		if inRemoved {
			continue
		}
		if w.deleteRemote(ctx, p) {
			w.forget(p)
			removed = append(removed, p)
		}
	}
}

// syncEntry sends one path, not what a folder holds. Unless force, a file
// or symlink the space already has as it is here is left alone. It reports
// whether the space now has the path as it is here.
func (w *mirrorWatcher) syncEntry(ctx context.Context, localAbs, rel string, info os.FileInfo, force bool) bool {
	target := ""
	if info.Mode()&os.ModeSymlink != 0 {
		var err error
		if target, err = os.Readlink(localAbs); err != nil {
			return false
		}
	}
	now := entryOf(info, target)
	if now.kind == kindNone {
		return false // sockets, pipes, devices — not uploadable
	}
	was, had := w.known[rel]
	if had && was.kind != now.kind {
		if !w.deleteRemote(ctx, rel) {
			return false
		}
		w.forget(rel)
		had = false
	}
	if had && !force && was == now {
		return true
	}
	if had && now.kind == kindFile && was.size == now.size && was.mtime == now.mtime && was.mode != now.mode {
		// Only the mode changed (chmod keeps size and mtime): set it,
		// rather than send the content again.
		return w.setMode(ctx, rel, now)
	}
	switch now.kind {
	case kindDir:
		// A new folder is made, empty or not, and one whose mode changed
		// is given it.
		return w.makeDir(ctx, rel, now)
	case kindLink:
		return w.uploadSymlink(ctx, rel, now)
	default:
		return w.uploadFile(ctx, localAbs, rel, info, now)
	}
}

// knowsAt reports whether anything was sent at rel or below it.
func (w *mirrorWatcher) knowsAt(rel string) bool {
	if _, ok := w.known[rel]; ok {
		return true
	}
	prefix := rel + "/"
	for p := range w.known {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// forget drops what was known to be in the space at rel and below.
func (w *mirrorWatcher) forget(rel string) {
	delete(w.known, rel)
	prefix := rel + "/"
	for p := range w.known {
		if strings.HasPrefix(p, prefix) {
			delete(w.known, p)
		}
	}
}

// uploadFile sends a single regular file to the space via the same chunked
// path as the initial mirror.
func (w *mirrorWatcher) uploadFile(ctx context.Context, localAbs, rel string, info os.FileInfo, e sentEntry) bool {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s (dry-run)\n", rel)
		}
		return true
	}
	u := upload{
		rel:      rel,
		localAbs: localAbs,
		size:     info.Size(),
		mtime:    info.ModTime(),
		mode:     uint32(info.Mode().Perm()),
	}
	if err := w.opts.uploadOne(ctx, u, w.buf); err != nil {
		w.fail(rel, err)
		return false
	}
	w.sent(rel, e)
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s\n", rel)
	}
	return true
}

// makeDir makes a folder on the space with its mode.
func (w *mirrorWatcher) makeDir(ctx context.Context, rel string, e sentEntry) bool {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s/ (dry-run)\n", rel)
		}
		return true
	}
	if err := w.opts.makeDir(ctx, rel, e.mode); err != nil {
		w.fail(rel, err)
		return false
	}
	w.sent(rel, e)
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s/\n", rel)
	}
	return true
}

// setMode sets a file's permission bits on the space.
func (w *mirrorWatcher) setMode(ctx context.Context, rel string, e sentEntry) bool {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s (mode %o, dry-run)\n", rel, e.mode)
		}
		return true
	}
	if err := w.opts.client.ChmodSpaceFile(ctx, w.opts.spaceID, path.Join(w.opts.remoteDir, rel), e.mode); err != nil {
		w.fail(rel, err)
		return false
	}
	w.sent(rel, e)
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s (mode %o)\n", rel, e.mode)
	}
	return true
}

// uploadSymlink creates a symlink on the remote.
func (w *mirrorWatcher) uploadSymlink(ctx context.Context, rel string, e sentEntry) bool {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s -> %s (dry-run)\n", rel, e.target)
		}
		return true
	}
	dest := path.Join(w.opts.remoteDir, rel)
	if err := w.opts.client.CreateSymlinkSpaceFile(ctx, w.opts.spaceID, dest, e.target); err != nil {
		w.fail(rel, err)
		return false
	}
	w.sent(rel, e)
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s -> %s\n", rel, e.target)
	}
	return true
}

// deleteRemote removes a path from the space, reporting whether it is gone.
// Recursive so a single call covers directory removal.
func (w *mirrorWatcher) deleteRemote(ctx context.Context, rel string) bool {
	if w.opts.dryRun {
		fmt.Fprintf(os.Stderr, "  - %s (dry-run)\n", rel)
		return true
	}
	dest := path.Join(w.opts.remoteDir, rel)
	resp, err := w.opts.client.DeleteSpaceFile(ctx, w.opts.spaceID, apiclient.DeleteFileRequest{
		Path:      dest,
		Recursive: true,
	})
	if err != nil {
		w.fail(rel, err)
		return false
	}
	delete(w.reported, rel)
	// Suppress output if the agent reported nothing removed — the path may
	// have been a temp file from an atomic save that was never uploaded.
	if resp != nil && resp.Removed > 0 && w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  - %s\n", rel)
	}
	return true
}

// sent records what the space now has at a path.
func (w *mirrorWatcher) sent(rel string, e sentEntry) {
	w.known[rel] = e
	delete(w.reported, rel)
}

// fail notes a path to try again, and says why, once for each new reason.
func (w *mirrorWatcher) fail(rel string, err error) {
	w.failed[rel] = true
	if errors.Is(err, context.Canceled) {
		return
	}
	w.report(rel, err)
}

// report shows an error, unless it is the one last shown for rel.
func (w *mirrorWatcher) report(rel string, err error) {
	msg := cmdutil.Describe(err)
	if w.reported[rel] == msg {
		return
	}
	w.reported[rel] = msg
	again := " (trying again)"
	if w.stopping {
		again = ""
	}
	if rel == "" {
		fmt.Fprintf(os.Stderr, "  ! mirroring again: %s%s\n", msg, again)
		return
	}
	fmt.Fprintf(os.Stderr, "  ! %s: %s%s\n", rel, msg, again)
}
