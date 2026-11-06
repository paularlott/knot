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

	// finalFlushFor bounds the last pass a watching sync makes as it is
	// stopped, sending the local changes not yet sent.
	finalFlushFor = 5 * time.Second
	// retryMin and retryMax bound the wait before a pass that failed, the
	// server restarting for example, is tried again, doubling each time.
	retryMin = 2 * time.Second
	retryMax = time.Minute
)

// eventFollower follows the server's event stream; tests replace it.
type eventFollower func(ctx context.Context, onEvent func(apiclient.Event), onState func(bool)) error

// watch runs passes as changes come, after the first, which ended with
// firstErr. w is the directory's watcher when it was started before the
// first pass, nil when the directory did not exist then.
func (s *syncer) watch(ctx context.Context, w dirWatcher, firstErr error, follow eventFollower) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	watchLocal := s.opt.mode != modeDown
	watchRemote := s.opt.mode != modeUp

	// A directory the first pass made was not watched while it ran: look at
	// all of it once, now that it is.
	rescan := false
	var fsEvents <-chan watchEvent
	if watchLocal {
		if w == nil {
			var err error
			if w, err = newDirWatcher(s); err != nil {
				return fmt.Errorf("watch %s: %w", s.opt.dir, err)
			}
			w.add(s.opt.dir)
			rescan = true
		}
		defer w.close()
		fsEvents = w.events()
	}
	defer s.saveHashes(true)

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
	remoteDue := false
	var settle <-chan time.Time
	// A pass that fails is tried again after a while, everything it was to
	// do, not left for the next change or look at everything.
	var retryIn time.Duration
	retry := func() <-chan time.Time {
		retryIn = min(max(2*retryIn, retryMin), retryMax)
		return time.After(retryIn)
	}
	if worthRetrying(firstErr) {
		rescan = true
		settle = retry()
	} else if rescan {
		settle = time.After(0)
	}
	lastPoll := time.Now()
	lastRescan := time.Now()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			// Stopped: send what changed here and has not been sent yet,
			// briefly, rather than leave it behind.
			if watchLocal {
				fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finalFlushFor)
				if err := s.flushLocal(fctx, w, fsEvents, dirty); err != nil {
					s.log.WithError(err).Warn("stopped before every local change was sent")
				}
				cancel()
			}
			return nil

		case reply := <-s.opt.flush:
			// Asked to send every local change now, as a space is about to
			// stop.
			var err error
			if watchLocal {
				err = s.flushLocal(ctx, w, fsEvents, dirty)
				dirty, rescan = make(map[string]string), false
				lastRescan = time.Now()
				if worthRetrying(err) {
					rescan = true
					settle = retry()
				}
			}
			reply <- err

		case ev, ok := <-fsEvents:
			if !ok {
				return nil
			}
			if s.onFsEvent(w, ev, dirty) {
				rescan = true
			}
			if settle == nil {
				settle = time.After(settleTime)
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
			// Events lost (a queue overflowed): look at everything.
			if w != nil && w.takeLost() {
				s.log.Debug("file system events were lost: looking at every file")
				rescan = true
			}
			if (remoteDue || rescan) && settle == nil {
				settle = time.After(0)
			}

		case <-settle:
			settle = nil
			ids, full, err := s.gather(ctx, dirty, rescan, remoteDue)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				// What was to be looked at still is, shortly.
				s.log.WithError(err).Warn("could not look for changes")
				s.passed(passCounts{}, err)
				settle = retry()
				continue
			}
			if remoteDue {
				lastPoll = time.Now()
			}
			if rescan {
				lastRescan = time.Now()
			}
			dirty, rescan, remoteDue = make(map[string]string), false, false
			if full {
				ids = nil
			} else if len(ids) == 0 {
				retryIn = 0
				continue
			}
			err = s.pass(ctx, ids)
			if err != nil && ctx.Err() == nil {
				s.log.WithError(err).Warn("sync pass incomplete")
			}
			s.passed(s.report(), err)
			if !worthRetrying(err) || ctx.Err() != nil {
				retryIn = 0
				continue
			}
			if full {
				rescan = true
			} else {
				for _, id := range ids {
					if rel := s.relFor(id); rel != "" {
						dirty[id] = rel
					}
				}
			}
			settle = retry()
		}
	}
}

// worthRetrying says whether a pass that ended with err has more to do than
// deletions it refused as too many, which wait on the user, not on time.
func worthRetrying(err error) bool {
	if err == nil {
		return false
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range j.Unwrap() {
			if worthRetrying(e) {
				return true
			}
		}
		return false
	}
	var m *massDeleteError
	return !errors.As(err, &m)
}

// flushLocal sends what changed locally and has not been synced: the
// changes seen and, looking at every file, any the watcher has not told of
// yet (FSEvents reports a moment late).
func (s *syncer) flushLocal(ctx context.Context, w dirWatcher, fsEvents <-chan watchEvent, dirty map[string]string) error {
	for drained := false; !drained; {
		select {
		case ev, ok := <-fsEvents:
			if !ok {
				drained = true
				continue
			}
			s.onFsEvent(w, ev, dirty)
		default:
			drained = true
		}
	}
	ids, full, err := s.gather(ctx, dirty, true, false)
	if err != nil {
		return err
	}
	if full {
		ids = nil
	} else if len(ids) == 0 {
		return nil
	}
	err = s.pass(ctx, ids)
	s.passed(s.report(), err)
	return err
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
		s.log.Debug("looking at every local file")
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
		if isDirId(id) {
			if d := s.statLocalDir(rel); d != nil {
				s.dirs[s.identity(d.rel)] = d
			} else {
				delete(s.dirs, id)
			}
		} else if l := s.statLocal(rel); l != nil {
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
	if isDirId(id) {
		if d := s.dirs[id]; d != nil {
			return d.rel
		}
		if m := s.markers[id]; m != nil {
			return m.rel
		}
	}
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
func (s *syncer) onFsEvent(w dirWatcher, ev watchEvent, dirty map[string]string) bool {
	if ev.all {
		return true
	}
	rel, err := filepath.Rel(s.opt.dir, ev.path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	rel = filepath.ToSlash(rel)
	if internalName(filepath.Base(ev.path)) {
		return false
	}
	info, statErr := os.Lstat(ev.path)
	if statErr == nil && info.IsDir() {
		if s.excluded(rel, true) {
			return false
		}
		// A new directory, one changed (its mode), or one whose events were
		// lost: watch it, and look at it and what it holds, without walking
		// the rest of the tree.
		w.add(ev.path)
		dirty[s.identity(rel+"/")] = rel + "/"
		s.walkNew(ev.path, dirty)
		if ev.below {
			s.dirtyBelow(rel, dirty)
		}
		return false
	}
	if s.excluded(rel, false) {
		return false
	}
	// A path that is gone may have been a directory: look again at what was
	// known below it, and stop watching it.
	if statErr != nil {
		s.dirtyBelow(rel, dirty)
		w.remove(ev.path)
		dirty[s.identity(rel+"/")] = rel + "/"
	}
	dirty[s.identity(rel)] = rel
	return false
}

// dirtyBelow notes every file and folder known below a folder, to look at
// again.
func (s *syncer) dirtyBelow(rel string, dirty map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	prefix := s.identity(rel) + "/"
	for id, l := range s.local {
		if strings.HasPrefix(id, prefix) {
			dirty[id] = l.rel
		}
	}
	for id, d := range s.dirs {
		if strings.HasPrefix(id, prefix) {
			dirty[id] = d.rel
		}
	}
}

// walkNew notes every file and folder below a directory that has just
// appeared.
func (s *syncer) walkNew(dir string, dirty map[string]string) {
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(s.opt.dir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if p != dir {
				if s.excluded(rel, true) {
					return filepath.SkipDir
				}
				dirty[s.identity(rel+"/")] = rel + "/"
			}
			return nil
		}
		if !internalName(d.Name()) && !s.excluded(rel, false) {
			dirty[s.identity(rel)] = rel
		}
		return nil
	})
}
