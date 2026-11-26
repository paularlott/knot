package nfsserver

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/log"
)

// A staged copy's journal, beside it, records what was done to the copy, so
// that writes the nfs-server took and had not yet saved outlive it: killed
// or crashed, it takes the copies back when it starts again and saves them.
//
// NFS lets a server lose writes it has not committed (UNSTABLE), and says
// how: a restarted server answers with a new write verifier, and the client,
// seeing it, sends the writes again. The macOS client does not always: after
// the nfs-server restarted it sent COMMIT for writes the server before it
// had taken, got the new verifier, and took the writes as saved, so a
// write fsync'd without error was lost. The copies and their journals are
// written before each write is answered, so a server that is killed has
// them all; the operating system keeps what was written for a process that
// dies. (A machine that loses power loses them, as it loses its own
// unsynced writes, and the client, rebooted with it, has none to commit.)
//
// The journal is a line of JSON per record: the copy as made (journalInit),
// then each change (journalChange). A record cut short by a crash is the
// last, and is ignored: the change it records was never answered.

// journalSuffix names a staged copy's journal: the copy's file name with it.
const journalSuffix = ".journal"

// stagingLock is the file in the staging folder its server holds locked.
const stagingLock = "lock"

// journalInit is a staged copy as made, the first record of its journal.
type journalInit struct {
	Bucket      string            `json:"bucket" msgpack:"bucket"`
	Key         string            `json:"key" msgpack:"key"`
	Size        int64             `json:"size" msgpack:"size"`
	Sparse      bool              `json:"sparse,omitempty" msgpack:"sparse"`
	Keep        int64             `json:"keep,omitempty" msgpack:"keep"`
	Base        *journalBase      `json:"base,omitempty" msgpack:"base"`
	ContentType string            `json:"contentType,omitempty" msgpack:"contentType"`
	Meta        map[string]string `json:"meta,omitempty" msgpack:"meta"`
	Mtime       time.Time         `json:"mtime" msgpack:"mtime"`
	FromETag    string            `json:"fromETag,omitempty" msgpack:"fromETag"`
}

// journalBase is what a copy keeps of the saved file it was made from.
type journalBase struct {
	ETag   string `json:"etag" msgpack:"etag"`
	SHA256 string `json:"sha256" msgpack:"sha256"`
	Size   int64  `json:"size" msgpack:"size"`
}

// journalChange is a change to a staged copy: a write (Op "w", of N bytes at
// Off), a truncation (Op "t", to Size) or attributes set (Op "m"). A
// truncation and attributes set record the copy's attributes then: a new
// file's are set as it is made, before the truncation that empties it.
type journalChange struct {
	Op    string            `json:"op" msgpack:"op"`
	Off   int64             `json:"off,omitempty" msgpack:"off"`
	N     int64             `json:"n,omitempty" msgpack:"n"`
	Size  int64             `json:"size,omitempty" msgpack:"size"`
	Meta  map[string]string `json:"meta,omitempty" msgpack:"meta"`
	Mtime time.Time         `json:"mtime" msgpack:"mtime"`
}

// startJournal begins a new copy's journal; e.mu is held.
func (e *dirtyFile) startJournal() error {
	j, err := os.OpenFile(e.f.Name()+journalSuffix, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	e.journal = j
	rec := journalInit{
		Bucket: e.bucket, Key: e.key, Size: e.size, Sparse: e.sparse, Keep: e.keep,
		ContentType: e.contentType, Meta: e.meta, Mtime: e.mtime, FromETag: e.fromETag,
	}
	if e.base != nil {
		rec.Base = &journalBase{ETag: e.base.ETag, SHA256: e.base.SHA256, Size: e.base.Size}
	}
	return e.record(rec)
}

// record adds a record to the copy's journal; e.mu is held. One write, so a
// crash leaves at most the record cut short.
func (e *dirtyFile) record(rec any) error {
	if e.journal == nil {
		return os.ErrClosed
	}
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = e.journal.Write(append(b, '\n'))
	return err
}

// endJournal removes the copy's journal, the copy being saved or dropped;
// e.mu is held. Removed first, so that a crash as the copy is let go never
// leaves a journal that would have it saved again.
func (e *dirtyFile) endJournal() {
	if e.journal == nil {
		return
	}
	e.journal.Close()
	os.Remove(e.journal.Name())
	e.journal = nil
}

// recover takes back the staged copies a previous nfs-server left in the
// staging folder, to be saved as any other: when a commit asks, or once
// their writes have stopped. It is done before any request is served, so
// writes that come after are made over them. Files there with no journal
// are what a copy being made left, and are removed.
func (s *staging) recover() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	logger := log.WithGroup("nfs")
	for _, de := range entries {
		name := de.Name()
		if de.IsDir() || name == stagingLock || strings.HasSuffix(name, journalSuffix) {
			continue
		}
		p := filepath.Join(s.dir, name)
		e, err := s.replay(p)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				logger.Error("dropping unsaved writes that could not be taken back", "file", name, "error", err)
			}
			os.Remove(p + journalSuffix)
			os.Remove(p)
			continue
		}
		k := bucketKey(e.bucket, e.key)
		if old := s.files[k]; old != nil {
			// Not made: a copy is begun only once the one before is saved,
			// and its journal gone. Kept, the newer.
			logger.Warn("two unsaved copies of one file; keeping the newer", "bucket", e.bucket, "key", e.key)
			older := old
			if e.mtime.Before(old.mtime) {
				older = e
			}
			older.mu.Lock()
			older.gone = true
			if older.timer != nil {
				older.timer.Stop()
			}
			older.endJournal()
			older.f.Close()
			os.Remove(older.f.Name())
			older.mu.Unlock()
			if older == e {
				continue
			}
		}
		s.files[k] = e
		logger.Info("took back unsaved writes left by the nfs-server before", "bucket", e.bucket, "key", e.key, "size", e.size)
		e.mu.Lock()
		e.since = time.Now()
		e.schedule(s.flushDelay)
		e.mu.Unlock()
	}
	// Journals whose copy is gone were of copies already saved.
	for _, de := range entries {
		if name := de.Name(); strings.HasSuffix(name, journalSuffix) {
			if _, err := os.Stat(filepath.Join(s.dir, strings.TrimSuffix(name, journalSuffix))); errors.Is(err, os.ErrNotExist) {
				os.Remove(filepath.Join(s.dir, name))
			}
		}
	}
}

// replay makes the staged copy at p again from its journal.
func (s *staging) replay(p string) (*dirtyFile, error) {
	data, err := os.ReadFile(p + journalSuffix)
	if err != nil {
		return nil, err
	}
	// Whole records only: one cut short by the crash has no line end.
	lines := strings.SplitAfter(string(data), "\n")
	whole := int64(0)
	next := func() ([]byte, bool) {
		for len(lines) > 0 {
			l := lines[0]
			lines = lines[1:]
			if !strings.HasSuffix(l, "\n") {
				return nil, false
			}
			whole += int64(len(l))
			return []byte(l), true
		}
		return nil, false
	}
	first, ok := next()
	if !ok {
		return nil, os.ErrNotExist // begun, never made
	}
	var init journalInit
	if err := json.Unmarshal(first, &init); err != nil || init.Key == "" {
		return nil, os.ErrNotExist
	}
	e := &dirtyFile{
		s: s, bucket: init.Bucket, key: init.Key, size: init.Size, sparse: init.Sparse, keep: init.Keep,
		contentType: init.ContentType, meta: init.Meta, mtime: init.Mtime, fromETag: init.FromETag,
	}
	if e.meta == nil {
		e.meta = map[string]string{}
	}
	if init.Base != nil {
		e.base = &Object{Key: init.Key, ETag: init.Base.ETag, SHA256: init.Base.SHA256, Size: init.Base.Size}
	}
	for {
		line, ok := next()
		if !ok {
			break
		}
		var c journalChange
		if json.Unmarshal(line, &c) != nil {
			whole -= int64(len(line))
			break
		}
		switch c.Op {
		case "w":
			e.gotData = true
			e.size = max(e.size, c.Off+c.N)
			if e.sparse {
				e.written.add(c.Off, c.Off+c.N)
			}
			e.mtime = c.Mtime
			e.meta[metaMtime] = apiclient.FormatMtime(c.Mtime)
		case "t":
			if e.sparse {
				e.written.trim(c.Size)
				e.keep = min(e.keep, c.Size)
			}
			e.size = c.Size
			if c.Meta != nil {
				e.meta = c.Meta
			}
			e.mtime = c.Mtime
			e.meta[metaMtime] = apiclient.FormatMtime(c.Mtime)
		case "m":
			e.meta = c.Meta
			if e.meta == nil {
				e.meta = map[string]string{}
			}
			e.mtime = c.Mtime
		}
	}
	if e.f, err = os.OpenFile(p, os.O_RDWR, 0); err != nil {
		return nil, err
	}
	// A whole copy is the file: what a write cut short left past the size
	// recorded is not part of it.
	if !e.sparse {
		if err := e.f.Truncate(e.size); err != nil {
			e.f.Close()
			return nil, err
		}
	}
	// What follows the last whole record goes, so new records follow it.
	if err := os.Truncate(p+journalSuffix, whole); err != nil {
		e.f.Close()
		return nil, err
	}
	if e.journal, err = os.OpenFile(p+journalSuffix, os.O_WRONLY|os.O_APPEND, 0); err != nil {
		e.f.Close()
		return nil, err
	}
	return e, nil
}
