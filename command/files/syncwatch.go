package command_files

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/paularlott/knot/apiclient"
)

// Watching keeps a sync running: local changes are seen by watching the
// directory, the bucket's by following the server's event stream and then
// its change feed for just what changed. Neither is trusted alone, so the
// bucket is asked for its changes regularly when the stream is down (and now
// and then when it is up), and the directory is walked again now and then.

const (
	// settleTime gathers a burst of changes, such as an editor's save
	// (write, rename) or a build, into one pass.
	settleTime = 300 * time.Millisecond
	// pollDown and pollLive are how often the bucket is asked for changes
	// with the event stream down and up.
	pollDown = 15 * time.Second
	pollLive = 5 * time.Minute
	// rescanEvery is how often the directory is walked in full.
	rescanEvery = 5 * time.Minute
)

// eventFollower follows the server's event stream; tests replace it.
type eventFollower func(ctx context.Context, onEvent func(apiclient.Event), onState func(bool)) error

func (s *syncer) watch(ctx context.Context, follow eventFollower) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	watchLocal := s.opt.mode != modeDown
	watchRemote := s.opt.mode != modeUp

	var fsw *fsnotify.Watcher
	var fsEvents <-chan fsnotify.Event
	var fsErrors <-chan error
	if watchLocal {
		var err error
		if fsw, err = fsnotify.NewWatcher(); err != nil {
			return fmt.Errorf("watch %s: %w", s.opt.dir, err)
		}
		defer fsw.Close()
		s.watchTree(fsw, s.opt.dir)
		fsEvents, fsErrors = fsw.Events, fsw.Errors
	}

	// The bucket's event stream says when to ask for its changes.
	remoteKick := make(chan struct{}, 1)
	kick := func() {
		select {
		case remoteKick <- struct{}{}:
		default:
		}
	}
	var live atomic.Bool
	if watchRemote && follow != nil {
		go func() {
			err := follow(ctx, func(ev apiclient.Event) {
				if ev.Type != "files:changed" {
					return
				}
				var p apiclient.FilesChangedPayload
				json.Unmarshal(ev.Payload, &p)
				if len(p.BucketIds) == 0 {
					kick()
					return
				}
				for _, id := range p.BucketIds {
					if id == s.bucketId {
						kick()
						return
					}
				}
			}, func(connected bool) {
				if connected {
					s.log.Debug("live updates connected")
				} else if ctx.Err() == nil {
					s.log.Debug("live updates lost: asking the server for changes regularly until they are back", "every", pollDown)
				}
				live.Store(connected)
				// Changes made while it was down are caught up on now.
				kick()
			})
			if errors.Is(err, apiclient.ErrEventsRefused) {
				s.log.Warn("the server refused live updates: asking it for changes regularly instead", "every", pollDown)
			}
		}()
	}

	s.log.Info("watching for changes (Ctrl+C to stop)", "dir", s.opt.dir, "bucket", remoteName(s.opt.bucket, s.opt.prefix))

	dirty := make(map[string]string) // identity -> rel, local paths to look at
	rescan := false
	remoteDue := false
	var settle <-chan time.Time
	lastPoll := time.Now()
	lastRescan := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil

		case ev, ok := <-fsEvents:
			if !ok {
				return nil
			}
			if s.onFsEvent(fsw, ev, dirty) {
				rescan = true
			}
			if settle == nil {
				settle = time.After(settleTime)
			}

		case err, ok := <-fsErrors:
			if !ok {
				return nil
			}
			// Events lost (the kernel queue overflowed): look at everything.
			if errors.Is(err, fsnotify.ErrEventOverflow) {
				rescan = true
				if settle == nil {
					settle = time.After(settleTime)
				}
			} else if err != nil {
				s.log.WithError(err).Warn("watching the directory")
			}

		case <-remoteKick:
			remoteDue = true
			if settle == nil {
				settle = time.After(settleTime)
			}

		case <-tick.C:
			every := pollDown
			if live.Load() {
				every = pollLive
			}
			if watchRemote && time.Since(lastPoll) >= every {
				remoteDue = true
			}
			if time.Since(lastRescan) >= rescanEvery {
				rescan = true
			}
			if (remoteDue || rescan) && settle == nil {
				settle = time.After(0)
			}

		case <-settle:
			settle = nil
			ids, full, err := s.gather(ctx, dirty, rescan, remoteDue)
			if remoteDue {
				lastPoll = time.Now()
			}
			if rescan {
				lastRescan = time.Now()
			}
			dirty, rescan, remoteDue = make(map[string]string), false, false
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				s.log.WithError(err).Warn("could not look for changes")
				continue
			}
			if full {
				ids = nil
			} else if len(ids) == 0 {
				continue
			}
			if err := s.pass(ctx, ids); err != nil && ctx.Err() == nil {
				s.log.WithError(err).Warn("sync pass incomplete")
			}
			s.report()
		}
	}
}

// gather brings both sides up to date for a pass: the local paths that
// changed (or the whole directory), and the bucket's changes. It returns the
// identities to look at, or full for all of them.
func (s *syncer) gather(ctx context.Context, dirty map[string]string, rescan, remote bool) ([]string, bool, error) {
	full := rescan
	seen := make(map[string]bool)
	if remote {
		ids, reset, err := s.pullChanges(ctx)
		if err != nil {
			return nil, false, err
		}
		full = full || reset
		for _, id := range ids {
			seen[id] = true
		}
	}
	if full {
		s.mu.Lock()
		err := s.scanLocal()
		s.mu.Unlock()
		if err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	// Remote changes are compared with the local file as it is now.
	for id := range seen {
		if rel := s.relFor(id); rel != "" {
			dirty[id] = rel
		}
	}
	for id, rel := range dirty {
		seen[id] = true
		s.mu.Lock()
		if l := s.statLocal(rel); l != nil {
			s.local[s.identity(l.rel)] = l
		} else {
			delete(s.local, id)
		}
		s.mu.Unlock()
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	return ids, false, nil
}

// relFor returns a local path for an identity: the local file's, else the
// bucket's.
func (s *syncer) relFor(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l := s.local[id]; l != nil {
		return l.rel
	}
	if r := s.remote[id]; r != nil {
		return r.rel
	}
	if b := s.base[id]; b != nil {
		return strings.TrimPrefix(b.Key, s.opt.prefix)
	}
	return ""
}

// onFsEvent notes what a file system event touched. It reports true when
// the change is better found by walking (a directory came or went).
func (s *syncer) onFsEvent(fsw *fsnotify.Watcher, ev fsnotify.Event, dirty map[string]string) bool {
	rel, err := filepath.Rel(s.opt.dir, ev.Name)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	if internalName(filepath.Base(ev.Name)) {
		return false
	}
	info, statErr := os.Lstat(ev.Name)
	if statErr == nil && info.IsDir() {
		if s.excluded(rel, true) {
			return false
		}
		// A new directory: watch it, and walk it for what it already holds.
		s.watchTree(fsw, ev.Name)
		return true
	}
	if s.excluded(rel, false) {
		return false
	}
	// A path that is gone may have been a directory: walk if anything was
	// known below it.
	if statErr != nil {
		s.mu.Lock()
		prefix := s.identity(rel) + "/"
		for id := range s.local {
			if strings.HasPrefix(id, prefix) {
				s.mu.Unlock()
				return true
			}
		}
		s.mu.Unlock()
	}
	dirty[s.identity(rel)] = rel
	return false
}

// watchTree watches a directory and those below it that are not excluded.
func (s *syncer) watchTree(fsw *fsnotify.Watcher, root string) {
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
		if err := fsw.Add(p); err != nil {
			s.log.WithError(err).Warn("cannot watch folder", "dir", s.rel(p))
		}
		return nil
	})
}
