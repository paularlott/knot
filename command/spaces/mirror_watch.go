package command_spaces

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/paularlott/knot/apiclient"
)

// debounceInterval is how long the watcher waits after the last filesystem
// event before processing the coalesced batch. Editors that save atomically
// (write temp → rename) produce a burst of 3-5 events in <50ms; this window
// collapses them into a single upload.
const debounceInterval = 300 * time.Millisecond

// watchDefaultExcludes are patterns always excluded in watch mode. Vim swap
// files (.*.swp) appear and disappear rapidly during edits — uploading them
// is pure noise.
var watchDefaultExcludes = []string{".*.swp", ".*.swo", ".*.swn"}

// runWatch performs the initial mirror, then watches the local tree for
// incremental changes and syncs them to the space. One-way: local → remote.
// Blocks until Ctrl+C / SIGTERM.
func (o *mirrorOptions) runWatch(ctx context.Context) error {
	// Add watch-specific excludes (vim swap files, etc.) so both the initial
	// mirror and the live watcher skip them.
	o.excludes = append(o.excludes, watchDefaultExcludes...)

	start := time.Now()
	stats, err := o.run(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Done in %s\n", stats.String(time.Since(start)))
	fmt.Fprintf(os.Stderr, "Watching for changes... (Ctrl+C to stop)\n")

	// Cancel-on-signal so the event loop exits cleanly on Ctrl+C. The parent
	// ctx comes from the CLI and is NOT signal-aware (the framework doesn't
	// install a handler).
	watchCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create file watcher: %w", err)
	}
	defer fsw.Close()

	w := &mirrorWatcher{
		opts:     o,
		excludes: compileExcludes(o.excludes),
		fsw:      fsw,
		pending:  make(map[string]fsnotify.Op),
	}

	// Seed: watch every non-excluded directory under localRoot.
	w.seedDirs(o.localRoot)

	ticker := time.NewTicker(debounceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-watchCtx.Done():
			w.flush(ctx)
			fmt.Fprintf(os.Stderr, "\nStopped watching.\n")
			return nil

		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			w.onEvent(event)

		case werr, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			if werr != nil {
				fmt.Fprintf(os.Stderr, "  ! watcher: %v\n", werr)
			}

		case <-ticker.C:
			w.flush(ctx)
		}
	}
}

// mirrorWatcher holds the fsnotify watcher and the pending-event buffer.
type mirrorWatcher struct {
	opts     *mirrorOptions
	excludes func(rel string, isDir bool) bool
	fsw      *fsnotify.Watcher
	mu       sync.Mutex
	pending  map[string]fsnotify.Op
}

// rel converts an absolute local path to a slash-relative path under localRoot.
// Returns "" for the root itself or paths outside the tree.
func (w *mirrorWatcher) rel(absPath string) string {
	rel, err := filepath.Rel(w.opts.localRoot, absPath)
	if err != nil {
		return ""
	}
	s := filepath.ToSlash(rel)
	if s == "." {
		return ""
	}
	return s
}

// seedDirs walks localRoot and adds every non-excluded directory to the
// watcher. Called once after the initial mirror.
func (w *mirrorWatcher) seedDirs(root string) {
	_ = w.fsw.Add(root)
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root || !d.IsDir() {
			return nil
		}
		rel := w.rel(p)
		if rel != "" && w.excludes(rel, true) {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(p); err != nil {
			fmt.Fprintf(os.Stderr, "  ! watch %s: %v\n", rel, err)
		}
		return nil
	})
}

// onEvent buffers a filesystem event for debounced processing.
func (w *mirrorWatcher) onEvent(event fsnotify.Event) {
	rel := w.rel(event.Name)
	if rel == "" {
		return
	}
	// For Remove/Rename the path is gone — stat fails, isDir unknown. That's
	// fine: ancestor exclude checks don't depend on isDir, and dir-only
	// patterns only shield new dirs (which a Remove can't be).
	isDir := false
	if info, err := os.Lstat(event.Name); err == nil {
		isDir = info.IsDir()
	}
	if w.excludes(rel, isDir) {
		return
	}
	w.mu.Lock()
	w.pending[rel] = w.pending[rel] | event.Op
	w.mu.Unlock()
}

// flush drains the pending buffer and syncs each path. Called by the ticker
// and once on shutdown.
func (w *mirrorWatcher) flush(ctx context.Context) {
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	pending := w.pending
	w.pending = make(map[string]fsnotify.Op)
	w.mu.Unlock()

	for rel := range pending {
		if ctx.Err() != nil {
			return
		}
		w.syncPath(ctx, rel)
	}
}

// syncPath brings a single local path into sync with the remote. If the path
// no longer exists locally it's deleted from the remote; if it's a directory
// its contents are uploaded; otherwise the file/symlink is uploaded.
func (w *mirrorWatcher) syncPath(ctx context.Context, rel string) {
	localAbs := filepath.Join(w.opts.localRoot, filepath.FromSlash(rel))

	info, err := os.Lstat(localAbs)
	if err != nil {
		// Path gone → delete from remote (recursive covers directories).
		w.deleteRemote(ctx, rel)
		return
	}

	// Re-check excludes — the path type may have changed since the event
	// was queued (e.g. a file replaced by a directory).
	if w.excludes(rel, info.IsDir()) {
		return
	}

	switch {
	case info.IsDir():
		// New directory: watch it, then upload everything inside.
		w.watchTree(localAbs)
		w.uploadDirContents(ctx, localAbs)

	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(localAbs)
		if err != nil {
			return
		}
		w.uploadSymlink(ctx, rel, target)

	default:
		// Skip sockets, pipes, devices — not uploadable.
		if !info.Mode().IsRegular() {
			return
		}
		w.uploadFile(ctx, localAbs, rel, info)
	}
}

// watchTree adds a directory and all its non-excluded subdirectories to the
// watcher. Used both at seed time and when a new directory appears.
func (w *mirrorWatcher) watchTree(root string) {
	_ = w.fsw.Add(root)
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == root || !d.IsDir() {
			return nil
		}
		rel := w.rel(p)
		if rel != "" && w.excludes(rel, true) {
			return filepath.SkipDir
		}
		if err := w.fsw.Add(p); err != nil {
			fmt.Fprintf(os.Stderr, "  ! watch %s: %v\n", rel, err)
		}
		return nil
	})
}

// uploadDirContents walks a local directory and uploads every non-excluded
// file and symlink to the remote. Called when a new directory is created.
func (w *mirrorWatcher) uploadDirContents(ctx context.Context, localAbs string) {
	filepath.WalkDir(localAbs, func(p string, d os.DirEntry, err error) error {
		if err != nil || p == localAbs {
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
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err == nil {
				w.uploadSymlink(ctx, rel, target)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil // sockets, pipes, devices
		}
		w.uploadFile(ctx, p, rel, info)
		return nil
	})
}

// uploadFile sends a single regular file to the space via the same chunked
// path as the initial mirror.
func (w *mirrorWatcher) uploadFile(ctx context.Context, localAbs, rel string, info os.FileInfo) {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s (dry-run)\n", rel)
		}
		return
	}
	u := upload{
		rel:      rel,
		localAbs: localAbs,
		size:     info.Size(),
		mtime:    info.ModTime(),
		mode:     uint32(info.Mode().Perm()),
	}
	if err := w.opts.uploadOne(ctx, u); err != nil {
		fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
		return
	}
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s\n", rel)
	}
}

// uploadSymlink creates a symlink on the remote.
func (w *mirrorWatcher) uploadSymlink(ctx context.Context, rel, target string) {
	if w.opts.dryRun {
		if w.opts.verbose {
			fmt.Fprintf(os.Stderr, "  ↑ %s -> %s (dry-run)\n", rel, target)
		}
		return
	}
	dest := path.Join(w.opts.remoteDir, rel)
	if err := w.opts.client.CreateSymlinkSpaceFile(ctx, w.opts.spaceID, dest, target); err != nil {
		fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
		return
	}
	if w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  ↑ %s -> %s\n", rel, target)
	}
}

// deleteRemote removes a path from the space. Recursive so a single call
// covers directory removal.
func (w *mirrorWatcher) deleteRemote(ctx context.Context, rel string) {
	if w.opts.dryRun {
		fmt.Fprintf(os.Stderr, "  - %s (dry-run)\n", rel)
		return
	}
	dest := path.Join(w.opts.remoteDir, rel)
	resp, err := w.opts.client.DeleteSpaceFile(ctx, w.opts.spaceID, apiclient.DeleteFileRequest{
		Path:      dest,
		Recursive: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
		return
	}
	// Suppress output if the agent reported nothing removed — the path may
	// have been a temp file from an atomic save that was never uploaded.
	if resp != nil && resp.Removed > 0 && w.opts.verbose {
		fmt.Fprintf(os.Stderr, "  - %s\n", rel)
	}
}
