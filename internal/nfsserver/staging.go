package nfsserver

import (
	"errors"
	"io"
	"maps"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/log"
)

// NFS writes a file a piece at a time, so writes go to a local copy of the
// file, answered as not yet stable. The copy is saved as the file when the
// client commits it, which clients do when the file is closed or synced; a
// client that never commits has it saved once writes stop for flushDelay, or
// at the latest maxDirtyAge after it first changed. Until then reads,
// listings and attributes come from the copy.
//
// A small file's copy is the whole file, and is saved whole. A file larger
// than sparseMin is changed in place: its copy holds only what was written,
// the rest is read from the saved file, and saving sends only what was
// written, so a change to a large file costs what the change does.

// A save that fails for a reason that may pass (the server unreachable or
// failing) is retried for as long as it takes, waiting a little longer each
// time up to retryMax, and one that met the file changing elsewhere again
// and again is retried soon; one the server refuses (quota, permission, a bucket
// gone, a bad name) is dropped, as retrying cannot help.
const (
	retryMin = 5 * time.Second
	// conflictRetry is how soon a save is tried again after the file kept
	// changing elsewhere as it was made, a little longer each time.
	conflictRetry = 250 * time.Millisecond
	retryMax      = time.Minute
)

// transient reports whether a save failed only for the file changing
// elsewhere as it was made, or its content not having arrived yet.
func transient(err error) bool {
	return errors.Is(err, errPrecondition) || errors.Is(err, errBusy)
}

// busyPatience is how long a save is tried again at once for a file that
// is changing elsewhere, before the client is told it failed.
const busyPatience = 10 * time.Second

// permanent reports whether a save failed for good.
func permanent(err error) bool {
	return errors.Is(err, syscall.EDQUOT) || errors.Is(err, os.ErrPermission) ||
		errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.EINVAL)
}

// sparseMin is the size above which a file is changed in place.
const sparseMin = 8 << 20

type staging struct {
	fs          *storeFS
	dir         string
	flushDelay  time.Duration
	maxDirtyAge time.Duration

	mu     sync.Mutex
	files  map[string]*dirtyFile // by bucketKey
	closed bool

	// failed holds why a file's writes were dropped, refused by the server,
	// until the client is told: its next WRITE or COMMIT fails with it,
	// rather than a COMMIT finding nothing to save and answering that all
	// is well, or a new copy of the saved file taking the dropped writes'
	// place unseen.
	failed map[string]error
}

// fail records that a file's writes were dropped for err.
func (s *staging) fail(k string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failed == nil {
		s.failed = map[string]error{}
	}
	s.failed[k] = err
}

// failure is why a file's writes were dropped, if they were; with clear,
// it is forgotten, the client having now been told.
func (s *staging) failure(bucket, key string, clear bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := bucketKey(bucket, key)
	err := s.failed[k]
	if clear {
		delete(s.failed, k)
	}
	return err
}

// dirtyFile is the local copy of a file with writes not yet saved. Its
// mutex is held while it is written, read or saved; never take staging.mu
// while holding it except in drop.
type dirtyFile struct {
	s           *staging
	bucket, key string

	mu          sync.Mutex
	f           *os.File
	size        int64
	base        *Object // the saved file the copy was made from, nil for a new one
	sparse      bool    // the copy holds only the spans written, see sparseMin
	written     spans   // what was written, for a sparse copy
	keep        int64   // how much of the saved file still holds, for a sparse copy: cut short, the rest is zeros
	mtime       time.Time
	contentType string
	meta        map[string]string
	since       time.Time // when it changed first since last saved
	timer       *time.Timer
	failures    int
	gone        bool // saved or removed; a new copy must be made
	// fromETag is the version of the saved file the copy began from, and
	// gotData whether anything has been written to the copy since.
	fromETag string
	gotData  bool
}

func bucketKey(bucket, key string) string { return bucket + "\x00" + key }

func newStaging(fs *storeFS, dir string, flushDelay, maxDirtyAge time.Duration) *staging {
	return &staging{fs: fs, dir: dir, flushDelay: flushDelay, maxDirtyAge: maxDirtyAge, files: map[string]*dirtyFile{}}
}

// get returns the staged copy of a file, nil when it has none.
func (s *staging) get(bucket, key string) *dirtyFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.files[bucketKey(bucket, key)]
}

// What a new staged copy starts from.
type loadMode int

const (
	loadNew     loadMode = iota // nothing: the file is known not to exist
	loadAttrs                   // the saved file's attributes, not its content
	loadContent                 // the saved file
)

// open returns the staged copy of a file, locked, making one when there is
// none from what load says.
func (s *staging) open(bucket, key string, load loadMode) (*dirtyFile, error) {
	k := bucketKey(bucket, key)
	// Writes on top of ones dropped would hide that they were: fail until
	// the client has been told (see failed), unless it starts the file
	// afresh.
	if load == loadContent {
		if err := s.failure(bucket, key, false); err != nil {
			return nil, err
		}
	} else {
		s.failure(bucket, key, true)
	}
	for {
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return nil, os.ErrClosed
		}
		e := s.files[k]
		if e == nil {
			e = &dirtyFile{s: s, bucket: bucket, key: key, meta: map[string]string{}}
			e.mu.Lock()
			s.files[k] = e
			s.mu.Unlock()
			if err := e.load(load); err != nil {
				e.gone = true
				if e.f != nil {
					e.f.Close()
					os.Remove(e.f.Name())
				}
				e.mu.Unlock()
				s.mu.Lock()
				if s.files[k] == e {
					delete(s.files, k)
				}
				s.mu.Unlock()
				return nil, err
			}
			return e, nil
		}
		s.mu.Unlock()

		e.mu.Lock()
		if !e.gone {
			return e, nil
		}
		e.mu.Unlock() // saved meanwhile: make a new copy
	}
}

// load fills a new copy.
func (e *dirtyFile) load(load loadMode) error {
	f, err := os.CreateTemp(e.s.dir, "stage-*")
	if err != nil {
		return err
	}
	e.f = f
	e.mtime = time.Now()

	switch load {
	case loadNew:
		e.meta[metaIno] = newIno()
		return nil
	case loadAttrs:
		if o, err := e.s.fs.backend.Head(ctx(), e.bucket, e.key); err == nil {
			e.keepAttrs(o)
			e.fromETag = o.ETag
		} else if errors.Is(err, os.ErrNotExist) {
			e.meta[metaIno] = newIno() // a new file
		} else {
			return err
		}
		return nil
	}
	o, err := e.s.fs.backend.Head(ctx(), e.bucket, e.key)
	if errors.Is(err, os.ErrNotExist) {
		e.meta[metaIno] = newIno()
		return nil
	} else if err != nil {
		return err
	}
	e.base = o
	e.keepAttrs(o)
	if o.Size > sparseMin {
		e.sparse, e.size, e.keep = true, o.Size, o.Size
		return nil
	}

	// A small file is copied whole: from the cache when it is there.
	var c io.ReadCloser
	if cf := e.s.fs.cache.open(o.SHA256); cf != nil {
		c = cf
	} else if c, o, err = e.s.fs.backend.Get(ctx(), e.bucket, e.key, ""); err != nil {
		return err
	}
	defer c.Close()
	n, err := io.Copy(f, c)
	if err != nil {
		return err
	}
	e.size = n
	e.base = o
	return nil
}

func (e *dirtyFile) keepAttrs(o *Object) {
	e.contentType = o.ContentType
	for k, v := range o.Meta {
		e.meta[k] = v
	}
	e.mtime = mtimeFrom(o.Meta, o.ModifiedAt)
}

// info describes the staged file, nil once it is saved.
func (e *dirtyFile) info(name string) *fileInfo {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gone {
		return nil
	}
	return (&fileInfo{name: name, size: e.size, mode: modeFrom(e.meta), mtime: e.mtime, id: fileId(e.bucket, e.key)}).withMeta(e.meta)
}

// readAt reads the staged content; ok is false once it is saved, when the
// saved file must be read instead.
func (e *dirtyFile) readAt(b []byte, off int64) (n int, err error, ok bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gone {
		return 0, nil, false
	}
	if off >= e.size {
		return 0, io.EOF, true
	}
	end := false
	if max := e.size - off; int64(len(b)) >= max {
		b, end = b[:max], true
	}
	if e.sparse {
		n, err = e.readSparse(b, off)
	} else {
		n, err = e.f.ReadAt(b, off)
	}
	if end && (err == nil || errors.Is(err, io.EOF)) {
		err = io.EOF
	}
	return n, err, true
}

// readSparse reads a sparse copy: what was written from the copy, the rest
// from the saved file, and zeros past its end; e.mu is held.
func (e *dirtyFile) readSparse(b []byte, off int64) (int, error) {
	n := 0
	for n < len(b) {
		pos := off + int64(n)
		inside, until := e.written.at(pos)
		chunk := min(int64(len(b)-n), until-pos)
		switch {
		case inside:
			if _, err := e.f.ReadAt(b[n:n+int(chunk)], pos); err != nil && !errors.Is(err, io.EOF) {
				return n, err
			}
		case pos < e.keep:
			chunk = min(chunk, e.keep-pos)
			// Any version: the save writes what was written here over
			// whatever the file is by then.
			got, err := e.s.fs.readSaved(e.bucket, e.key, e.base.SHA256, "", e.base.Size, b[n:n+int(chunk)], pos)
			if err != nil && !errors.Is(err, io.EOF) {
				return n, err
			}
			if int64(got) < chunk {
				clear(b[n+got : n+int(chunk)])
			}
		default:
			clear(b[n : n+int(chunk)])
		}
		n += int(chunk)
	}
	return n, nil
}

// writeAt writes to the staged copy; e.mu is held.
func (e *dirtyFile) writeAt(b []byte, off int64) (int, error) {
	e.gotData = true
	n, err := e.f.WriteAt(b, off)
	if end := off + int64(n); end > e.size {
		e.size = end
	}
	if e.sparse {
		e.written.add(off, off+int64(n))
	}
	e.modified()
	return n, err
}

// truncate sets the staged copy's size; e.mu is held.
func (e *dirtyFile) truncate(size int64) error {
	if err := e.f.Truncate(size); err != nil {
		return err
	}
	if e.sparse {
		e.written.trim(size)
		// What is cut is gone: grown back, by a truncate or by a write past
		// it, it is zeros, not the saved bytes.
		e.keep = min(e.keep, size)
	}
	e.size = size
	e.modified()
	return nil
}

// setMeta records an attribute on the staged copy, false once it is saved.
func (e *dirtyFile) setMeta(kv map[string]string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gone {
		return false
	}
	for k, v := range kv {
		e.meta[k] = v
	}
	e.mtime = mtimeFrom(e.meta, e.mtime)
	e.changed()
	return true
}

// modified records a change to the content, which makes now the file's
// modification time until one is set; e.mu is held.
func (e *dirtyFile) modified() {
	e.mtime = time.Now()
	e.meta[metaMtime] = apiclient.FormatMtime(e.mtime)
	e.changed()
}

// changed marks the copy changed and schedules saving it; e.mu is held.
func (e *dirtyFile) changed() {
	now := time.Now()
	if e.since.IsZero() {
		e.since = now
	}
	e.schedule(e.s.flushDelay)
}

func (e *dirtyFile) schedule(d time.Duration) {
	if rem := e.s.maxDirtyAge - time.Since(e.since); rem < d {
		d = max(rem, 0)
	}
	if e.timer == nil {
		e.timer = time.AfterFunc(d, func() { e.s.flushIdle(e) })
	} else {
		e.timer.Reset(d)
	}
}

// flushIdle saves a copy whose writes have stopped for a while, as clients
// that never commit need. A saved file only emptied, which a client does
// as it opens one to write it afresh, waits for what is written to it, for
// a commit, or for maxDirtyAge: its writes can be a while coming (queued
// behind others on a busy client), and saved meanwhile, others would read
// the file empty.
func (s *staging) flushIdle(e *dirtyFile) {
	e.mu.Lock()
	wait := !e.gone && e.onlyEmptied() && time.Since(e.since) < s.maxDirtyAge
	if wait {
		e.schedule(s.maxDirtyAge - time.Since(e.since))
	}
	e.mu.Unlock()
	if !wait {
		s.flush(e)
	}
}

// onlyEmptied reports whether the copy is a saved file emptied and nothing
// more; e.mu is held.
func (e *dirtyFile) onlyEmptied() bool {
	return !e.gotData && e.size == 0 && e.fromETag != ""
}

// staleTruncation reports whether the copy is a saved file emptied and
// nothing more, which another client has since replaced or removed: as on a
// local disk, truncating a file that is no longer there makes nothing, and
// saving it would empty the file that replaced it. A client is left with
// such a copy when its handle went stale between emptying the file and
// writing it, its writes refused. e.mu is held.
func (e *dirtyFile) staleTruncation() bool {
	if !e.onlyEmptied() {
		return false
	}
	o, err := e.s.fs.backend.Head(ctx(), e.bucket, e.key)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	return err == nil && o.ETag != e.fromETag
}

// mergeNotice is how long after saving changes over a newer version the
// file is recorded again (see noticeMerge).
var mergeNotice = time.Second

// noticeMerge records a file saved over a newer version again, unchanged,
// a moment after: a new version of it, which its clients see as a change
// made elsewhere. The version saved holds what others wrote as well as
// this client's changes, but this client learns of it in the answer to its
// own save, takes it for its own change, and goes on serving what it has
// cached of the rest, its own earlier writes that the newer version did not
// keep among it.
//
// It gets a modification time of its own too: NFSv3 clients take a file's
// cached content to be current while its size and modification time are
// as they were.
func (fs *storeFS) noticeMerge(bucket, key string, saved *Object) {
	time.AfterFunc(mergeNotice, func() {
		// Only the file as saved gets the new time: one changed since,
		// even only its mode or owner, keeping the same content, is left
		// as it is, as is one written again, the same, meanwhile.
		cur, err := fs.backend.Head(ctx(), bucket, key)
		if err != nil || cur.ETag != saved.ETag || !maps.Equal(cur.Meta, saved.Meta) {
			return
		}
		meta := maps.Clone(cur.Meta)
		if meta == nil {
			meta = map[string]string{}
		}
		meta[metaMtime] = apiclient.FormatMtime(time.Now())
		if err := fs.backend.SetMeta(ctx(), bucket, key, meta, cur.ETag); err == nil {
			fs.forget(bucket, key, false)
		}
	})
}

// flush saves a staged copy as the file and drops it.
func (s *staging) flush(e *dirtyFile) error {
	// A file changing elsewhere as it is saved, or whose content has not
	// yet reached the server, is tried again at once for a while: the
	// client waiting on the save has its close() fail otherwise. The copy
	// is let go between tries, so others can look at the file meanwhile.
	start, pause := time.Now(), 100*time.Millisecond
	for {
		e.mu.Lock()
		if e.gone {
			e.mu.Unlock()
			return nil
		}
		if e.staleTruncation() {
			log.WithGroup("nfs").Info("not saving the truncation of a file replaced or removed since", "bucket", e.bucket, "key", e.key)
			s.drop(e)
			e.mu.Unlock()
			return nil
		}
		err := e.save()
		if err == nil {
			s.drop(e)
			e.mu.Unlock()
			return nil
		}
		if transient(err) && time.Since(start) < busyPatience {
			e.mu.Unlock()
			time.Sleep(pause)
			pause = min(pause*2, time.Second)
			continue
		}
		defer e.mu.Unlock()
		return s.retryLater(e, err)
	}
}

// busyFlushes is how many flushes in a row a file changing elsewhere, or
// waiting for its content, is tried again soon after; then it waits as for
// any other failure, as it is not settling (a server whose content it
// waits on gone, say).
const busyFlushes = 5

// retryLater handles a save that failed: dropped if it never can be made, else
// tried again later; e.mu is held.
func (s *staging) retryLater(e *dirtyFile, err error) error {
	e.failures++
	logger := log.WithGroup("nfs")
	if permanent(err) {
		logger.Error("dropping unsaved writes the server refused", "bucket", e.bucket, "key", e.key, "error", err)
		s.drop(e)
		s.fail(bucketKey(e.bucket, e.key), err)
		return err
	}
	var wait time.Duration
	if transient(err) && e.failures <= busyFlushes {
		// The file kept changing elsewhere as the change was made: busy,
		// not failing, so soon tried again.
		wait = min(conflictRetry*time.Duration(e.failures), retryMin)
		logger.Info("file changed elsewhere while saving, will retry", "bucket", e.bucket, "key", e.key, "attempt", e.failures, "in", wait)
	} else {
		wait = min(retryMin<<min(e.failures-1, 8), retryMax)
		logger.Warn("saving file failed, will retry", "bucket", e.bucket, "key", e.key, "error", err, "attempt", e.failures, "in", wait)
	}
	e.since = time.Now()
	e.schedule(wait)
	return err
}

// save writes the staged copy to file storage: a sparse copy as a patch of
// what was written, any other whole; e.mu is held.
func (e *dirtyFile) save() error {
	// The file as saved is what the server answers: kept, so the lookups
	// that follow a save need not ask again.
	var saved *Object
	defer func() {
		e.s.fs.forget(e.bucket, e.key, false)
		if saved != nil {
			e.s.fs.remember(e.bucket, e.key, objectInfo(path.Base(e.key), e.bucket, saved))
			e.s.fs.noteExists(e.bucket, e.key)
		}
	}()
	if e.sparse {
		// What was written, and everything past what still holds of the
		// saved file: the server fills nothing in (zeros where the file was
		// cut short, or grew with nothing written, are sent as such).
		send := append(spans(nil), e.written...)
		send.add(e.keep, e.size)
		patch := func(ifMatch string) error {
			ranges := make([]Range, 0, len(send))
			for _, sp := range send {
				// Past the copy's end (nothing written there) is zeros.
				n := sp.end - sp.off
				data := io.LimitReader(io.MultiReader(io.NewSectionReader(e.f, sp.off, n), zeros{}), n)
				ranges = append(ranges, Range{Offset: sp.off, Length: n, Data: data})
			}
			o, err := e.s.fs.backend.Patch(ctx(), e.bucket, e.key, ranges, e.size, ifMatch, e.contentType, e.meta)
			saved = o
			return err
		}
		err := patch(e.base.ETag)
		if errors.Is(err, errPrecondition) {
			// Changed elsewhere since: what was written here goes over
			// that, rather than being lost. The server makes the change
			// again itself if the file changes again as it does.
			log.WithGroup("nfs").Info("file changed while being written, applying the writes over the newer version", "bucket", e.bucket, "key", e.key)
			// What is saved is now neither this client's file nor the
			// newer one, so it gets a time of its own: with the time this
			// client already has, and its size, its client would take
			// it for the file it holds and keep what it read before.
			e.mtime = time.Now()
			e.meta[metaMtime] = apiclient.FormatMtime(e.mtime)
			// A newer version shorter than the one written over has
			// nothing past its end: what is not written here there is
			// zeros, as in a file grown, and sent, as the server fills in
			// nothing. Refused for being short of that, it shrank again.
			for range 3 {
				if cur, err := e.s.fs.backend.Head(ctx(), e.bucket, e.key); err == nil && cur.Size < e.keep {
					send.add(cur.Size, e.keep)
				}
				if err = patch(""); !errors.Is(err, syscall.EINVAL) {
					break
				}
			}
			if err == nil && saved != nil {
				e.s.fs.noticeMerge(e.bucket, e.key, saved)
			}
		}
		return err
	}

	o, err := e.s.fs.backend.Put(ctx(), e.bucket, e.key, io.NewSectionReader(e.f, 0, e.size), e.size, e.contentType, e.meta)
	saved = o
	if err == nil && e.s.fs.cache.keeps(e.size) {
		// The copy is the file now saved: keep it.
		e.f.Close()
		e.s.fs.cache.adopt(e.f.Name(), o.SHA256, e.size)
		e.f = nil
	}
	return err
}

// zeros reads as endless zero bytes.
type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// drop forgets a staged copy; e.mu is held.
func (s *staging) drop(e *dirtyFile) {
	e.gone = true
	if e.timer != nil {
		e.timer.Stop()
	}
	if e.f != nil {
		e.f.Close()
		os.Remove(e.f.Name())
	}
	s.mu.Lock()
	if k := bucketKey(e.bucket, e.key); s.files[k] == e {
		delete(s.files, k)
	}
	s.mu.Unlock()
}

// remove discards a file's staged copy, reporting whether it had one.
func (s *staging) remove(bucket, key string) bool {
	e := s.get(bucket, key)
	if e == nil {
		return false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.gone {
		return false
	}
	s.drop(e)
	return true
}

// matching returns the staged copies whose bucketKey is k or starts with
// prefix.
func (s *staging) matching(k, prefix string) []*dirtyFile {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*dirtyFile
	for fk, e := range s.files {
		if fk == k || (prefix != "" && strings.HasPrefix(fk, prefix)) {
			out = append(out, e)
		}
	}
	return out
}

// flushUnder saves now the staged file with bucketKey k and those below
// prefix.
func (s *staging) flushUnder(k, prefix string) error {
	for _, e := range s.matching(k, prefix) {
		if err := s.flush(e); err != nil {
			return err
		}
	}
	return nil
}

// hasUnder reports whether any file below prefix in bucket is staged.
func (s *staging) hasUnder(bucket, prefix string) bool {
	return len(s.matching("", bucketKey(bucket, prefix))) > 0
}

// list describes the staged entries directly in the folder prefix of
// bucket: files staged there and folders holding staged files.
func (s *staging) list(bucket, prefix string) map[string]os.FileInfo {
	out := map[string]os.FileInfo{}
	for _, e := range s.matching("", bucketKey(bucket, prefix)) {
		rest := e.key[len(prefix):]
		if d, _, ok := strings.Cut(rest, "/"); ok {
			out[d] = dirInfo(d, bucket, prefix+d+"/", time.Now())
		} else if fi := e.info(path.Base(rest)); fi != nil {
			out[rest] = fi
		}
	}
	return out
}

// close saves every staged file and refuses new writes.
func (s *staging) close() {
	s.mu.Lock()
	s.closed = true
	files := make([]*dirtyFile, 0, len(s.files))
	for _, e := range s.files {
		files = append(files, e)
	}
	s.mu.Unlock()
	for _, e := range files {
		if s.flush(e) != nil {
			e.mu.Lock()
			if !e.gone {
				log.WithGroup("nfs").Error("dropping unsaved writes at shutdown", "bucket", e.bucket, "key", e.key)
				s.drop(e)
			}
			e.mu.Unlock()
		}
	}
}
