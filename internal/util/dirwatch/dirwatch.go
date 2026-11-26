// Package dirwatch reports changes below a directory: with fsnotify watching
// each folder, or on macOS with FSEvents watching the whole tree in one
// stream, where kqueue would need an open file for every file and folder.
//
// Events are hints, not a full account: when they may have been lost (a
// queue overflowed) the watcher says so, and the caller looks at everything.
package dirwatch

import (
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
)

// Watcher reports changes below the watched directory.
type Watcher interface {
	// Add watches a folder and those below it, for a watcher that needs
	// each one added.
	Add(dir string)
	// Remove stops watching a folder that is gone.
	Remove(dir string)
	// Events reports the paths that changed.
	Events() <-chan Event
	// TakeLost reports, once, that events were lost and everything must be
	// looked at.
	TakeLost() bool
	Close()
}

// Event is a change below the directory.
type Event struct {
	Path  string
	Below bool // events below Path were lost: look at all of it
	All   bool // events were lost: look at everything
}

// Options says what to watch and where problems go.
type Options struct {
	// Root is the directory watched.
	Root string
	// Excluded reports whether a folder, slash-separated below Root, is not
	// watched. Nil watches every folder.
	Excluded func(rel string, isDir bool) bool
	// Warn reports a problem the watcher carries on past. Nil ignores it.
	Warn func(msg string, err error)
	// Unwatched reports that Add could not watch failed of the total folders
	// it was given, most often because the system's limit on watches or open
	// files is reached; LimitHint says why and how to fix it. Changes in
	// those folders are not reported. Nil ignores it.
	Unwatched func(failed, total int, firstErr error)
	// Failed, when set, is told of each folder Add could not watch, for
	// debugging; Unwatched still sums them up.
	Failed func(dir string, err error)
	// Label names the FSEvents dispatch queue, for debugging.
	Label string
}

func (o Options) warn(msg string, err error) {
	if o.Warn != nil {
		o.Warn(msg, err)
	}
}

// New watches opt.Root: with FSEvents on macOS, else, or when FSEvents cannot
// be started, with fsnotify. The caller then calls Add(opt.Root).
func New(opt Options) (Watcher, error) {
	return newPlatform(opt)
}

// notifyWatcher watches each folder with fsnotify: inotify on Linux,
// ReadDirectoryChangesW on Windows, kqueue on macOS when FSEvents is not to
// be had.
type notifyWatcher struct {
	opt  Options
	fsw  *fsnotify.Watcher
	ch   chan Event
	lost atomic.Bool
}

// NewNotify watches with fsnotify, adding each folder.
func NewNotify(opt Options) (Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	w := &notifyWatcher{opt: opt, fsw: fsw, ch: make(chan Event, 1024)}
	go func() {
		for {
			select {
			case ev, ok := <-fsw.Events:
				if !ok {
					return
				}
				w.ch <- Event{Path: ev.Name}
			case err, ok := <-fsw.Errors:
				if !ok {
					return
				}
				if errors.Is(err, fsnotify.ErrEventOverflow) {
					w.lost.Store(true)
				} else if err != nil {
					opt.warn("watching the directory", err)
				}
			}
		}
	}()
	return w, nil
}

func (w *notifyWatcher) Events() <-chan Event { return w.ch }
func (w *notifyWatcher) TakeLost() bool       { return w.lost.Swap(false) }
func (w *notifyWatcher) Close()               { w.fsw.Close() }
func (w *notifyWatcher) Remove(dir string)    { w.fsw.Remove(dir) }

// Add watches a directory and those below it that are not excluded. Folders
// that cannot be watched are reported together through opt.Unwatched.
func (w *notifyWatcher) Add(root string) {
	failed, total := 0, 0
	var firstErr error
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if p != w.opt.Root && w.opt.Excluded != nil {
			if rel, rerr := filepath.Rel(w.opt.Root, p); rerr == nil && w.opt.Excluded(filepath.ToSlash(rel), true) {
				return filepath.SkipDir
			}
		}
		total++
		if err := w.fsw.Add(p); err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			if w.opt.Failed != nil {
				w.opt.Failed(p, err)
			}
		}
		return nil
	})
	if failed > 0 && w.opt.Unwatched != nil {
		w.opt.Unwatched(failed, total, firstErr)
	}
}
