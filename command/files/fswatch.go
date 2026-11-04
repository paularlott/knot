package command_files

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

// dirWatcher reports changes below the synced directory: fsnotify watching
// each folder, or on macOS FSEvents watching the whole tree in one stream.
type dirWatcher interface {
	// add watches a folder and those below it, for a watcher that needs
	// each one added.
	add(dir string)
	// remove stops watching a folder that is gone.
	remove(dir string)
	// events reports the paths that changed.
	events() <-chan watchEvent
	// takeLost reports, once, that events were lost and everything must be
	// looked at.
	takeLost() bool
	close()
}

// watchEvent is a change below the directory.
type watchEvent struct {
	path  string
	below bool // events below path were lost: look at all of it
	all   bool // events were lost: look at everything
}

// notifyWatcher watches each folder with fsnotify: inotify on Linux,
// ReadDirectoryChangesW on Windows, kqueue on macOS when FSEvents is not to
// be had.
type notifyWatcher struct {
	s    *syncer
	fsw  *fsnotify.Watcher
	ch   chan watchEvent
	lost atomic.Bool
}

func newNotifyWatcher(s *syncer) (*notifyWatcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &notifyWatcher{s: s, fsw: fsw, ch: make(chan watchEvent, 1024)}
	go func() {
		for {
			select {
			case ev, ok := <-fsw.Events:
				if !ok {
					return
				}
				w.ch <- watchEvent{path: ev.Name}
			case err, ok := <-fsw.Errors:
				if !ok {
					return
				}
				if errors.Is(err, fsnotify.ErrEventOverflow) {
					w.lost.Store(true)
				} else if err != nil {
					s.log.WithError(err).Warn("watching the directory")
				}
			}
		}
	}()
	return w, nil
}

func (w *notifyWatcher) events() <-chan watchEvent { return w.ch }
func (w *notifyWatcher) takeLost() bool            { return w.lost.Swap(false) }
func (w *notifyWatcher) close()                    { w.fsw.Close() }
func (w *notifyWatcher) remove(dir string)         { w.fsw.Remove(dir) }
func (w *notifyWatcher) add(dir string)            { w.s.watchTree(w.fsw, dir) }

// watchTree watches a directory and those below it that are not excluded.
// Folders that cannot be watched, most often because the system's limit on
// watches or open files is reached, are reported once: changes in them are
// found by the full scan every rescanEvery instead.
func (s *syncer) watchTree(fsw *fsnotify.Watcher, root string) {
	failed, total := 0, 0
	var firstErr error
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != s.opt.dir {
			rel, _ := filepath.Rel(s.opt.dir, p)
			if s.excluded(filepath.ToSlash(rel), true) {
				return filepath.SkipDir
			}
		}
		total++
		if err := fsw.Add(p); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			s.log.WithError(err).Debug("cannot watch folder", "dir", s.rel(p))
		}
		return nil
	})
	if failed > 0 {
		s.warnOnce("watch-limit", "cannot watch every folder: changes in the others are found by a full scan every "+rescanEvery.String(),
			"unwatched", failed, "of", total, "reason", watchLimitHint(firstErr, total))
	}
}
