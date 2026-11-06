package command_files

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// The sync engine brings a local directory and a bucket folder into step, in
// one direction or both. A pass looks at some paths (all of them, or those a
// watcher saw change), decides what each needs, and does it.
//
// Files are compared by size and SHA-256. A local file's checksum is
// remembered with its size and modification time, so an unchanged file is
// never read again. Two ways, the state both sides last agreed on (the base)
// is what tells a file changed on one side from one changed on the other, a
// deletion from a creation, and a conflict from either.

type syncMode int

const (
	modeUp     syncMode = iota // local → bucket
	modeDown                   // bucket → local
	modeTwoWay                 // both ways
)

const (
	// massDeleteMin and massDeleteShare: a pass deleting more than this many
	// files, and more than this share of the files on that side, is held
	// back as more likely a mistake than a wish.
	massDeleteMin   = 10
	massDeleteShare = 0.5
	// recentFor is how long content that left the bucket is offered for
	// reuse: within the server's hold, with time to spare.
	recentFor = 50 * time.Minute
	// transferWorkers is how many files are sent or fetched at once.
	transferWorkers = 4
)

type syncOptions struct {
	mode            syncMode
	dir             string // the local directory, symlinks resolved
	bucket          string // as given
	prefix          string // the bucket folder, "" or ending in "/"
	del             bool
	dryRun          bool
	excludes        func(rel string, isDir bool) bool
	allowMassDelete bool
	statePath       string        // two ways: where the base is kept
	hashCachePath   string        // where local checksums are kept between runs; the default when empty
	log             logger.Logger // where the sync reports; the "sync" group of the knot logger by default
	watching        bool          // passes run on as changes come; no summary when nothing happened
	// onPass, when set, is told of each pass as it ends: what it did and
	// whether it was complete. A failed look for changes is a pass that
	// did nothing, with its error.
	onPass func(c passCounts, err error)
	// flush, when set, asks a watching sync to send every local change now;
	// the error, nil when all went, is sent back on the channel given.
	flush chan chan error
}

type remoteFile struct {
	key      string // full key in the bucket
	rel      string // below the prefix
	size     int64
	sha      string
	etag     string
	modified time.Time
	mode     os.FileMode // permission bits kept with it, 0 when none
	link     bool        // a symbolic link, its target the content
	meta     map[string]string
}

type localFile struct {
	rel    string // slash-separated, below the directory, as the file system spells it
	size   int64
	mtime  time.Time
	mode   os.FileMode // permission bits, 0 where the platform has none or for a link
	link   bool        // a symbolic link: target is its content
	target string
	sha    string    // "" until needed
	stamp  fileStamp // inode and change time, where the platform has them
}

type hashed struct {
	size  int64
	mtime time.Time
	stamp fileStamp
	sha   string
}

type syncer struct {
	opt    syncOptions
	client *apiclient.ApiClient

	bucketId string
	// own are the sync's own files inside the directory (see ownFiles).
	own      []string
	fullName string // the bucket's full name
	fold     bool   // the local file system ignores case

	log      logger.Logger
	mu       sync.Mutex
	remote   map[string]*remoteFile // by identity
	local    map[string]*localFile  // by identity, as last seen
	dirs     map[string]*localDir   // local folders, by identity ending in /
	markers  map[string]*remoteFile // bucket folders' markers, by identity ending in /
	base     map[string]*baseEntry  // two ways, by identity
	baseFile *recordFile[baseEntry] // where base is kept
	hashes   map[string]hashed      // by local rel
	recent   map[string]recentContent
	cursor   string
	warned   map[string]string
	baseDirt bool

	hashFile  *recordFile[hashCacheEntry] // where hashes are kept
	hashDirt  bool                        // hashes changed since the cache was written
	hashSaved time.Time                   // when it was
	hashedNow int                         // files read for their checksum this pass

	counts passCounts
}

type recentContent struct {
	key  string
	seen time.Time
}

type passCounts struct {
	uploaded, downloaded, unchanged, modes, deleted, conflicts, skipped, folders int
}

func (c passCounts) any() bool {
	return c.uploaded+c.downloaded+c.modes+c.deleted+c.conflicts+c.skipped+c.folders > 0
}

func newSyncer(client *apiclient.ApiClient, opt syncOptions) *syncer {
	if opt.log == nil {
		opt.log = log.WithGroup("sync")
	}
	if opt.excludes == nil {
		opt.excludes = func(string, bool) bool { return false }
	}
	return &syncer{
		opt:     opt,
		log:     opt.log,
		client:  client,
		remote:  make(map[string]*remoteFile),
		local:   make(map[string]*localFile),
		dirs:    make(map[string]*localDir),
		markers: make(map[string]*remoteFile),
		base:    make(map[string]*baseEntry),
		hashes:  make(map[string]hashed),
		recent:  make(map[string]recentContent),
		warned:  make(map[string]string),
	}
}

// warnOnce logs a warning about a path once while it stays the same.
func (s *syncer) warnOnce(id, msg string, keysAndValues ...any) {
	if s.warned[id] == msg {
		return
	}
	s.warned[id] = msg
	s.log.Warn(msg, keysAndValues...)
}

// rel is a local path as it is shown: below the synced directory.
func (s *syncer) rel(local string) string {
	if r, err := filepath.Rel(s.opt.dir, local); err == nil {
		return filepath.ToSlash(r)
	}
	return local
}

// identity is the name two spellings of one file share: the path itself, or,
// on a file system that ignores case, the path in lower case.
func (s *syncer) identity(rel string) string {
	if s.fold {
		return strings.ToLower(rel)
	}
	return rel
}

// internalName reports names the sync itself writes, never synced.
func internalName(name string) bool {
	return strings.HasPrefix(name, ".knot-download-") || strings.HasPrefix(name, ".knot-case-") || strings.HasPrefix(name, ".knot-link-")
}

// excluded reports whether a path is left out of the sync.
func (s *syncer) excluded(rel string, isDir bool) bool {
	if !isDir {
		for _, own := range s.own {
			if rel == own || strings.HasPrefix(rel, own+".tmp-") {
				return true
			}
		}
	}
	return internalName(path.Base(rel)) || s.opt.excludes(rel, isDir)
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

// prepare checks both sides and learns what the sync needs of them.
func (s *syncer) prepare(ctx context.Context) error {
	info, err := os.Stat(s.opt.dir)
	switch {
	case err == nil && !info.IsDir():
		return fmt.Errorf("%s is not a directory", s.opt.dir)
	case os.IsNotExist(err) && s.opt.mode != modeUp:
		if !s.opt.dryRun {
			if err := os.MkdirAll(s.opt.dir, 0755); err != nil {
				return err
			}
		}
	case err != nil:
		return err
	}
	if real, err := filepath.EvalSymlinks(s.opt.dir); err == nil {
		s.opt.dir = real
	}
	s.fold = caseInsensitive(s.opt.dir)

	b, err := s.client.GetFileBucket(ctx, s.opt.bucket)
	if err != nil {
		return apiError(err)
	}
	s.bucketId, s.fullName = b.Id, b.Name
	if s.opt.mode != modeDown && b.Access == "read" {
		return fmt.Errorf("%s is shared with you for reading only", s.opt.bucket)
	}
	if s.opt.mode == modeTwoWay {
		if err := s.loadBase(b.Name); err != nil {
			return err
		}
	}
	if err := s.loadHashes(); err != nil {
		return err
	}
	s.own = ownFiles(s.opt.dir, s.opt.hashCachePath, s.opt.statePath)
	if !s.opt.dryRun {
		if sd, err := syncDir(); err == nil {
			sweepSyncDir(sd, time.Now())
		}
	}
	return nil
}

// ownFiles returns, of the sync's own files, those inside the directory
// synced, as paths below it: a checksum cache or state kept there (as
// --hash-file or --state-file can put it) is not synced itself.
func ownFiles(dir string, paths ...string) []string {
	var own []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		parent := filepath.Dir(p)
		if real, err := filepath.EvalSymlinks(parent); err == nil {
			parent = real
		}
		rel, err := filepath.Rel(dir, filepath.Join(parent, filepath.Base(p)))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		own = append(own, filepath.ToSlash(rel))
	}
	return own
}

// caseInsensitive reports whether dir's file system ignores case, by making
// a file and looking for it under another spelling.
func caseInsensitive(dir string) bool {
	f, err := os.CreateTemp(dir, ".knot-case-")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	base := filepath.Base(name)
	other := strings.ToUpper(base)
	if other == base {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, other))
	return err == nil
}

// ---------------------------------------------------------------------------
// The bucket side
// ---------------------------------------------------------------------------

// loadRemote reads the bucket folder in full. With feed set it uses the
// change feed, which returns a cursor to follow it with afterwards.
func (s *syncer) loadRemote(ctx context.Context, feed bool) error {
	remote := make(map[string]*remoteFile)
	markers := make(map[string]*remoteFile)
	var files []*remoteFile
	if feed {
		cursor := ""
		for {
			page, err := s.client.ListFileChanges(ctx, s.opt.bucket, s.opt.prefix, cursor, 0)
			if err != nil {
				return apiError(err)
			}
			for _, c := range page.Changes {
				if !c.Deleted {
					files = append(files, fromInfo(c.FileObjectInfo, s.opt.prefix))
				}
			}
			cursor = page.Cursor
			if !page.More {
				break
			}
		}
		s.cursor = cursor
	} else {
		objects, _, err := listAll(ctx, s.client, s.opt.bucket, s.opt.prefix, "")
		if err != nil {
			return err
		}
		for _, o := range objects {
			files = append(files, fromInfo(o, s.opt.prefix))
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].key < files[j].key })
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range files {
		if f.rel == "" || strings.HasSuffix(f.rel, "/") {
			s.addMarker(markers, f)
			continue
		}
		s.addRemote(remote, f)
	}
	s.remote, s.markers = remote, markers
	for _, f := range remote {
		s.noteContent(f)
	}
	return nil
}

func fromInfo(o apiclient.FileObjectInfo, prefix string) *remoteFile {
	return &remoteFile{
		key:      o.Key,
		rel:      strings.TrimPrefix(o.Key, prefix),
		size:     o.Size,
		sha:      o.SHA256,
		etag:     o.ETag,
		modified: o.ModifiedAt,
		mode:     remoteMode(o.Meta),
		link:     isLinkMeta(o.Meta),
		meta:     o.Meta,
	}
}

// isLinkMeta reports whether a bucket file is a symbolic link.
func isLinkMeta(meta map[string]string) bool {
	mode, ok := apiclient.ParseFileMode(meta)
	return ok && mode&os.ModeSymlink != 0
}

// newLocal describes a local file or link from its Lstat, nil when it is
// neither or is a link that cannot be read.
func (s *syncer) newLocal(rel string, info os.FileInfo) *localFile {
	if info.Mode()&os.ModeSymlink != 0 {
		if !linksSupported {
			s.warnOnce("links", "symbolic links are not synced on this system", "file", rel)
			return nil
		}
		target, err := os.Readlink(filepath.Join(s.opt.dir, filepath.FromSlash(rel)))
		if err != nil {
			return nil
		}
		return &localFile{rel: rel, size: int64(len(target)), mtime: info.ModTime(), link: true, target: target, stamp: stampOf(info)}
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	return &localFile{rel: rel, size: info.Size(), mtime: info.ModTime(), mode: apiclient.LocalFileAttrs(info).Mode, stamp: stampOf(info)}
}

// remoteMode is the permission bits kept with a bucket file, 0 when it has
// none or the platform has none.
func remoteMode(meta map[string]string) os.FileMode {
	mode, _ := storedMode(meta)
	return mode
}

// addRemote adds a bucket file to remote, unless it is excluded, would land
// outside the directory, or differs only in case from one already there on a
// file system that cannot hold both.
func (s *syncer) addRemote(remote map[string]*remoteFile, f *remoteFile) {
	if s.excluded(f.rel, false) {
		return
	}
	if _, err := within(s.opt.dir, f.rel); err != nil {
		s.warnOnce("outside:"+f.rel, "skipped: the name would land outside the directory", "file", remoteName(s.opt.bucket, f.key))
		return
	}
	id := s.identity(f.rel)
	if have, ok := remote[id]; ok && have.key != f.key {
		s.warnOnce("case:"+id, "skipped: the name differs only in case from another, and this file system cannot hold both",
			"file", remoteName(s.opt.bucket, f.key), "other", remoteName(s.opt.bucket, have.key))
		return
	}
	remote[id] = f
}

// noteContent remembers that the bucket holds content, for reuse. The caller
// holds s.mu.
func (s *syncer) noteContent(f *remoteFile) {
	if f.sha != "" {
		s.recent[f.sha] = recentContent{key: f.key, seen: time.Now()}
	}
}

// pullChanges follows the change feed, returning the identities that
// changed. reset is set when the feed had to be read again in full.
func (s *syncer) pullChanges(ctx context.Context) (ids []string, reset bool, err error) {
	if s.cursor == "" {
		if err := s.loadRemote(ctx, true); err != nil {
			return nil, false, err
		}
		return nil, true, nil
	}
	for {
		page, err := s.client.ListFileChanges(ctx, s.opt.bucket, s.opt.prefix, s.cursor, 0)
		if err != nil {
			return nil, false, apiError(err)
		}
		if page.Reset {
			if err := s.loadRemote(ctx, true); err != nil {
				return nil, false, err
			}
			return nil, true, nil
		}
		s.mu.Lock()
		for _, c := range page.Changes {
			f := fromInfo(c.FileObjectInfo, s.opt.prefix)
			if f.rel == "" || strings.HasSuffix(f.rel, "/") {
				if f.rel == "" {
					continue
				}
				id := s.identity(f.rel)
				if c.Deleted {
					if have, ok := s.markers[id]; ok && have.key == f.key {
						delete(s.markers, id)
					}
				} else {
					s.addMarker(s.markers, f)
				}
				ids = append(ids, id)
				continue
			}
			id := s.identity(f.rel)
			if c.Deleted {
				if have, ok := s.remote[id]; ok && have.key == f.key {
					s.noteContent(have)
					delete(s.remote, id)
				}
			} else {
				if have, ok := s.remote[id]; ok && have.key != f.key {
					// Another spelling of a file held already.
					s.addRemote(s.remote, f)
					continue
				}
				delete(s.remote, id)
				s.addRemote(s.remote, f)
				s.noteContent(f)
			}
			ids = append(ids, id)
		}
		s.cursor = page.Cursor
		s.mu.Unlock()
		if !page.More {
			return ids, false, nil
		}
	}
}

// ---------------------------------------------------------------------------
// The local side
// ---------------------------------------------------------------------------

// scanLocal walks the directory in full.
func (s *syncer) scanLocal() error {
	local := make(map[string]*localFile)
	dirs := make(map[string]*localDir)
	err := filepath.WalkDir(s.opt.dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == s.opt.dir {
				return err
			}
			return nil // gone while walking, or unreadable: next pass
		}
		if p == s.opt.dir {
			return nil
		}
		rel, err := filepath.Rel(s.opt.dir, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if s.excluded(rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if info, err := d.Info(); err == nil {
				dirs[s.identity(rel+"/")] = newLocalDir(rel+"/", info)
			}
			return nil
		}
		if !d.Type().IsRegular() && d.Type()&fs.ModeSymlink == 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if l := s.newLocal(rel, info); l != nil {
			local[s.identity(rel)] = l
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.local, s.dirs = local, dirs
	return nil
}

// statLocal looks at one path afresh, nil when there is no file to sync there.
func (s *syncer) statLocal(rel string) *localFile {
	if s.excluded(rel, false) {
		return nil
	}
	// Never through a link: the file would be another, outside the directory.
	if linkOnPath(s.opt.dir, rel) != "" {
		return nil
	}
	info, err := os.Lstat(filepath.Join(s.opt.dir, filepath.FromSlash(rel)))
	if err != nil {
		return nil
	}
	// On a file system that ignores case, keep the spelling it uses.
	if s.fold {
		if real := s.spelling(rel); real != "" {
			rel = real
		}
	}
	for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
		if s.excluded(dir, true) {
			return nil
		}
	}
	return s.newLocal(rel, info)
}

// spelling returns how the file system spells a path, "" if it cannot tell.
func (s *syncer) spelling(rel string) string {
	parts := strings.Split(rel, "/")
	dir := s.opt.dir
	for i, part := range parts {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		found := ""
		for _, e := range entries {
			if e.Name() == part {
				found = part
				break
			}
			if found == "" && strings.EqualFold(e.Name(), part) {
				found = e.Name()
			}
		}
		if found == "" {
			return ""
		}
		parts[i] = found
		dir = filepath.Join(dir, found)
	}
	return strings.Join(parts, "/")
}

// localSHA returns a local file's checksum, reading it only when its size,
// modification time or stamp changed since it was last read.
func (s *syncer) localSHA(l *localFile) (string, error) {
	if l.link {
		sum := sha256.Sum256([]byte(l.target))
		return hex.EncodeToString(sum[:]), nil
	}
	s.mu.Lock()
	if l.sha != "" {
		s.mu.Unlock()
		return l.sha, nil
	}
	if h, ok := s.hashes[l.rel]; ok && h.size == l.size && h.mtime.Equal(l.mtime) && stampMatches(h.stamp, l.stamp) {
		l.sha = h.sha
		s.mu.Unlock()
		return l.sha, nil
	}
	if b := s.base[s.identity(l.rel)]; b != nil && b.matchesLocal(l) {
		l.sha = b.SHA
		s.mu.Unlock()
		return l.sha, nil
	}
	s.mu.Unlock()

	sha, err := fileSHA256(filepath.Join(s.opt.dir, filepath.FromSlash(l.rel)))
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	l.sha = sha
	s.hashes[l.rel] = hashed{size: l.size, mtime: l.mtime, stamp: l.stamp, sha: sha}
	s.hashDirt = true
	s.hashedNow++
	s.mu.Unlock()
	return sha, nil
}

func (s *syncer) sameContent(l *localFile, r *remoteFile) bool {
	if l == nil || r == nil || l.size != r.size || l.link != r.link {
		return false
	}
	sha, err := s.localSHA(l)
	return err == nil && sha == r.sha
}

// unchangedSince reports whether the local file is as it was seen: the file
// is not replaced or removed under an edit made since.
func (s *syncer) unchangedSince(rel string, l *localFile) bool {
	info, err := os.Lstat(filepath.Join(s.opt.dir, filepath.FromSlash(rel)))
	if l == nil {
		return os.IsNotExist(err)
	}
	return err == nil && info.Size() == l.size && info.ModTime().Equal(l.mtime) && stampMatches(l.stamp, stampOf(info))
}

// ---------------------------------------------------------------------------
// Planning
// ---------------------------------------------------------------------------

type actKind int

const (
	actNone         actKind = iota
	actUpload               // send the local file
	actDownload             // fetch the bucket's file
	actDeleteLocal          // remove the local file
	actDeleteRemote         // remove the bucket's file
	actConflict             // changed on both sides: keep both
	actAgree                // the same on both sides: note it in the base
	actForget               // gone from both sides: drop it from the base
)

type action struct {
	kind actKind
	id   string
	l    *localFile
	r    *remoteFile
	b    *baseEntry
	// dir: the action is a folder's, d the local folder and r its marker.
	dir bool
	d   *localDir
	// keepTimes: the bucket's file matches and only its time is brought over.
	keepTimes bool
	// setMode: the content matches but the modes differ; the mode is copied
	// to the bucket (modeToRemote) or the local file (modeToLocal).
	setMode modeFix
}

type modeFix int

const (
	modeAsIs modeFix = iota
	modeToRemote
	modeToLocal
)

// plan decides what each path needs.
func (s *syncer) plan(ids []string) []action {
	var out []action
	for _, id := range ids {
		l, r, b := s.local[id], s.remote[id], s.base[id]
		a := action{id: id, l: l, r: r, b: b}
		switch s.opt.mode {
		case modeUp:
			switch {
			case l != nil && s.sameContent(l, r):
				a.kind = actAgree
				if l.mode != 0 && l.mode != r.mode {
					a.setMode = modeToRemote
				}
			case l != nil:
				a.kind = actUpload
			case r != nil && s.opt.del:
				a.kind = actDeleteRemote
			}
		case modeDown:
			switch {
			case r != nil && s.sameContent(l, r):
				a.kind = actAgree
				a.keepTimes = !r.modified.IsZero() && l.mtime.Sub(r.modified).Abs() > time.Second
				if l.mode != 0 && r.mode != 0 && l.mode != r.mode {
					a.setMode = modeToLocal
				}
			case r != nil:
				a.kind = actDownload
			case l != nil && s.opt.del:
				a.kind = actDeleteLocal
			}
		case modeTwoWay:
			a.kind = s.planTwoWay(l, r, b)
			if a.kind == actAgree {
				a.setMode = planModeTwoWay(l, r, b)
			}
		}
		if a.kind != actNone {
			out = append(out, a)
		}
	}
	return out
}

func (s *syncer) planTwoWay(l *localFile, r *remoteFile, b *baseEntry) actKind {
	localIsBase := l != nil && b != nil && s.matchesBase(l, b)
	remoteIsBase := r != nil && b != nil && r.sha == b.SHA
	switch {
	case l == nil && r == nil:
		if b != nil {
			return actForget
		}
		return actNone
	case l != nil && r != nil && s.sameContent(l, r):
		return actAgree
	case b == nil:
		switch {
		case r == nil:
			return actUpload
		case l == nil:
			return actDownload
		}
		return actConflict
	case l == nil:
		// Deleted here.
		if remoteIsBase {
			if s.opt.del {
				return actDeleteRemote
			}
			return actDownload // put back
		}
		return actDownload // changed there since: the change wins
	case r == nil:
		// Deleted there.
		if localIsBase {
			if s.opt.del {
				return actDeleteLocal
			}
			return actUpload // put back
		}
		return actUpload // changed here since: the change wins
	case localIsBase && !remoteIsBase:
		return actDownload
	case remoteIsBase && !localIsBase:
		return actUpload
	}
	return actConflict
}

// planModeTwoWay decides where a file's mode goes when both sides hold the
// same content but not the same mode.
func planModeTwoWay(l *localFile, r *remoteFile, b *baseEntry) modeFix {
	var base os.FileMode
	if b != nil {
		base = os.FileMode(b.Mode)
	}
	return twoWayMode(l.mode, r.mode, base)
}

// twoWayMode decides where the mode goes when the two sides hold a file or
// folder with different modes: to the bucket when it keeps none, else from
// the side that changed it since the base, this side when both did. With no
// base mode to go by, the bucket's is the one set on purpose.
func twoWayMode(local, remote, base os.FileMode) modeFix {
	if local == 0 || local == remote {
		return modeAsIs
	}
	if remote == 0 {
		return modeToRemote
	}
	if base == 0 || local == base {
		return modeToLocal
	}
	return modeToRemote
}

func (s *syncer) matchesBase(l *localFile, b *baseEntry) bool {
	if b.matchesLocal(l) {
		return true
	}
	sha, err := s.localSHA(l)
	return err == nil && sha == b.SHA
}

// guardDeletes holds back a pass's deletions on a side when they look like
// a mistake: most of the files there, or everything because the other side
// came up empty (a directory not mounted, a share withdrawn).
func (s *syncer) guardDeletes(plan []action) ([]action, error) {
	if s.opt.allowMassDelete {
		return plan, nil
	}
	var delLocal, delRemote int
	for _, a := range plan {
		if a.dir {
			continue // folders go with the files: held back when they are
		}
		switch a.kind {
		case actDeleteLocal:
			delLocal++
		case actDeleteRemote:
			delRemote++
		}
	}
	check := func(n, total, other int, where string) string {
		if n == 0 {
			return ""
		}
		if (n > massDeleteMin && float64(n) > massDeleteShare*float64(total)) || (other == 0 && n > 1) {
			return fmt.Sprintf("refusing to delete %d of the %d files in %s: run with --allow-mass-delete if that is what you want", n, total, where)
		}
		return ""
	}
	msgs := []string{
		check(delLocal, len(s.local), len(s.remote), s.opt.dir),
		check(delRemote, len(s.remote), len(s.local), remoteName(s.opt.bucket, s.opt.prefix)),
	}
	var refused []string
	for _, m := range msgs {
		if m != "" {
			refused = append(refused, m)
		}
	}
	if len(refused) == 0 {
		return plan, nil
	}
	kept := plan[:0:0]
	for _, a := range plan {
		if (a.kind == actDeleteLocal && msgs[0] != "") || (a.kind == actDeleteRemote && msgs[1] != "") {
			continue
		}
		kept = append(kept, a)
	}
	return kept, errors.New(strings.Join(refused, "; "))
}

// ---------------------------------------------------------------------------
// Doing
// ---------------------------------------------------------------------------

// pass plans and applies the changes for ids, or every path when ids is nil.
// A refused mass deletion is returned as errMassDelete, after the rest of
// the pass is done.
func (s *syncer) pass(ctx context.Context, ids []string) error {
	if ids == nil {
		seen := make(map[string]bool)
		for id := range s.local {
			seen[id] = true
		}
		for id := range s.remote {
			seen[id] = true
		}
		for id := range s.base {
			seen[id] = true
		}
		for id := range s.dirs {
			seen[id] = true
		}
		for id := range s.markers {
			seen[id] = true
		}
		for id := range seen {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	var fileIds, dirIds []string
	for _, id := range ids {
		if isDirId(id) {
			dirIds = append(dirIds, id)
		} else {
			fileIds = append(fileIds, id)
		}
	}
	plan := append(s.plan(fileIds), s.planDirs(dirIds)...)
	plan, guardErr := s.guardDeletes(plan)
	if guardErr != nil && !s.opt.dryRun && !s.opt.watching {
		// A single pass changes nothing when it would delete too much.
		return guardErr
	}

	// Content is sent before anything is deleted, so a rename finds the
	// content still in the bucket and copies it rather than sending it.
	// Links are downloaded after the files, one at a time, so that a file
	// below a link's name lands in a real folder and never where the link
	// points.
	var transfers, links, deletes, rest, dirs []action
	for _, a := range plan {
		if a.dir {
			dirs = append(dirs, a)
			continue
		}
		switch a.kind {
		case actDownload, actConflict:
			if a.r != nil && a.r.link {
				links = append(links, a)
			} else {
				transfers = append(transfers, a)
			}
		case actUpload:
			transfers = append(transfers, a)
		case actDeleteLocal, actDeleteRemote:
			deletes = append(deletes, a)
		default:
			rest = append(rest, a)
		}
	}
	for _, a := range rest {
		s.apply(ctx, a)
	}
	errs := s.runParallel(ctx, transfers, transferWorkers)
	for _, a := range links {
		if err := s.apply(ctx, a); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range deletes {
		if err := s.apply(ctx, a); err != nil {
			errs = append(errs, err)
		}
	}
	errs = append(errs, s.applyDirs(ctx, dirs)...)
	if err := s.saveBase(); err != nil {
		errs = append(errs, err)
	}
	s.saveHashes(!s.opt.watching)
	if guardErr != nil {
		errs = append(errs, guardErr)
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (s *syncer) runParallel(ctx context.Context, actions []action, workers int) []error {
	var mu sync.Mutex
	var errs []error
	sem := make(chan struct{}, workers)
	var wg sync.WaitGroup
	for _, a := range actions {
		if ctx.Err() != nil {
			break
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(a action) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := s.apply(ctx, a); err != nil {
				mu.Lock()
				errs = append(errs, err)
				mu.Unlock()
			}
		}(a)
	}
	wg.Wait()
	return errs
}

// apply carries out one action. It is called from several goroutines.
func (s *syncer) apply(ctx context.Context, a action) error {
	if a.dir {
		return s.applyDir(ctx, a)
	}
	switch a.kind {
	case actAgree:
		// A link's own time cannot be set: Chtimes would set its target's.
		if a.keepTimes && !a.l.link && !s.opt.dryRun {
			local := filepath.Join(s.opt.dir, filepath.FromSlash(a.l.rel))
			os.Chtimes(local, a.r.modified, a.r.modified)
			if info, err := os.Stat(local); err == nil {
				a.l.mtime = info.ModTime()
			}
		}
		if a.setMode != modeAsIs {
			return s.copyMode(ctx, a)
		}
		s.mu.Lock()
		s.counts.unchanged++
		s.mu.Unlock()
		s.agree(a.id, a.l, a.r)
		return nil
	case actForget:
		s.mu.Lock()
		delete(s.base, a.id)
		s.baseDirt = true
		s.mu.Unlock()
		return nil
	case actUpload:
		return s.upload(ctx, a)
	case actDownload:
		return s.download(ctx, a)
	case actDeleteLocal:
		return s.deleteLocal(a)
	case actDeleteRemote:
		return s.deleteRemote(ctx, a)
	case actConflict:
		return s.conflict(ctx, a)
	}
	return nil
}

// agree records that both sides hold the same file.
func (s *syncer) agree(id string, l *localFile, r *remoteFile) {
	if s.opt.mode != modeTwoWay || s.opt.dryRun || l == nil || r == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.base[id] = &baseEntry{Key: r.key, SHA: r.sha, Size: r.size, LocalSize: l.size, LocalMtime: l.mtime.UnixNano(), LocalIno: l.stamp.ino, LocalCtime: l.stamp.ctime, Mode: uint32(l.mode)}
	s.baseDirt = true
}

// copyMode brings a file's mode across when only it differs: to the bucket,
// rewriting the file's metadata in place (its content is not copied), or to
// the local file.
func (s *syncer) copyMode(ctx context.Context, a action) error {
	l, r := a.l, a.r
	local := filepath.Join(s.opt.dir, filepath.FromSlash(l.rel))
	if a.setMode == modeToLocal {
		if s.opt.dryRun {
			s.log.Info("would set mode", "file", l.rel, "mode", fmt.Sprintf("%04o", r.mode))
			s.count(func(c *passCounts) { c.modes++ })
			return nil
		}
		if !s.unchangedSince(l.rel, l) {
			return nil
		}
		if err := os.Chmod(local, r.mode); err != nil {
			return err
		}
		s.log.Info("set mode", "file", l.rel, "mode", fmt.Sprintf("%04o", r.mode))
		nl := *l
		nl.mode = r.mode
		s.mu.Lock()
		s.local[a.id] = &nl
		s.counts.modes++
		s.mu.Unlock()
		s.agree(a.id, &nl, r)
		return nil
	}

	name := remoteName(s.opt.bucket, r.key)
	if s.opt.dryRun {
		s.log.Info("would set mode", "file", name, "mode", fmt.Sprintf("%04o", l.mode))
		s.count(func(c *passCounts) { c.modes++ })
		return nil
	}
	meta := make(map[string]string, len(r.meta)+1)
	for k, v := range r.meta {
		meta[k] = v
	}
	meta[apiclient.FileModeMeta] = apiclient.FormatFileMode(l.mode)
	info, err := s.client.CopyFileObject(ctx, apiclient.FileCopyRequest{
		SourceBucket: s.opt.bucket, SourceKey: r.key, DestBucket: s.opt.bucket, DestKey: r.key,
		IfMatch: r.etag, Meta: meta,
	})
	if err != nil {
		if apiclient.IsPreconditionFailed(err) {
			s.log.Debug("changed in the bucket meanwhile: looking again", "file", name)
			return nil
		}
		return fmt.Errorf("%s: %w", name, cmdutil.CleanErr(err))
	}
	s.log.Info("set mode", "file", name, "mode", fmt.Sprintf("%04o", l.mode))
	nr := fromInfo(*info, s.opt.prefix)
	if nr.sha == "" {
		nr.sha = r.sha
	}
	s.mu.Lock()
	s.remote[a.id] = nr
	s.counts.modes++
	s.mu.Unlock()
	s.agree(a.id, l, nr)
	return nil
}

func (s *syncer) upload(ctx context.Context, a action) error {
	l := a.l
	key := s.opt.prefix + l.rel
	if a.r != nil {
		key = a.r.key // keep the bucket's spelling of the name
	}
	local := filepath.Join(s.opt.dir, filepath.FromSlash(l.rel))
	if s.opt.dryRun {
		s.log.Info("would upload", "file", l.rel, "to", remoteName(s.opt.bucket, key))
		s.count(func(c *passCounts) { c.uploaded++ })
		return nil
	}
	sha, err := s.localSHA(l)
	if err != nil {
		return nil // gone since: the next pass sees it
	}
	// Two ways, the write is only made against the version planned against,
	// so a change made there meanwhile is never overwritten.
	ifMatch, ifAbsent := "", false
	if s.opt.mode == modeTwoWay {
		if a.r != nil {
			ifMatch = a.r.etag
		} else {
			ifAbsent = true
		}
	}
	info, how, err := s.send(ctx, local, key, l, sha, ifMatch, ifAbsent)
	if err != nil {
		if apiclient.IsPreconditionFailed(err) {
			s.log.Debug("changed in the bucket meanwhile: looking again", "file", remoteName(s.opt.bucket, key))
			return nil
		}
		return fmt.Errorf("%s: %w", local, cmdutil.CleanErr(err))
	}
	if how != "" {
		s.log.Info("uploaded", "file", l.rel, "to", remoteName(s.opt.bucket, key), "size", formatBytes(l.size), "content", how)
	} else {
		s.log.Info("uploaded", "file", l.rel, "to", remoteName(s.opt.bucket, key), "size", formatBytes(l.size))
	}
	r := fromInfo(*info, s.opt.prefix)
	if r.sha == "" {
		r.sha = sha
	}
	s.mu.Lock()
	s.remote[a.id] = r
	s.counts.uploaded++
	s.noteContent(r)
	s.mu.Unlock()
	s.agree(a.id, l, r)
	return nil
}

// send writes a local file to the bucket, without sending its content when
// the bucket has it: as a copy of a file that holds it, or by reusing content
// the bucket held until moments ago (a rename, or a change undone).
func (s *syncer) send(ctx context.Context, local, key string, l *localFile, sha, ifMatch string, ifAbsent bool) (*apiclient.FileObjectInfo, string, error) {
	// The bucket file to copy is the one last seen holding the content,
	// if it still does: looked up, not searched for, as a first sync of a
	// large tree would otherwise look through every file for each it sends.
	s.mu.Lock()
	var source string
	rc, recent := s.recent[sha]
	if recent && rc.key != key {
		if r := s.remote[s.identity(strings.TrimPrefix(rc.key, s.opt.prefix))]; r != nil && r.key == rc.key && r.sha == sha {
			source = r.key
		}
	}
	s.mu.Unlock()

	attrs := apiclient.FileAttrs{Mtime: l.mtime, Mode: l.mode}
	if l.link {
		attrs.Mode = linkMode
	}
	if source != "" {
		// The copy takes the local file's attributes, not the source's.
		meta := map[string]string{"mtime": apiclient.FormatMtime(l.mtime)}
		if attrs.Mode != 0 {
			meta[apiclient.FileModeMeta] = apiclient.FormatFileMode(attrs.Mode)
		}
		info, err := s.client.CopyFileObject(ctx, apiclient.FileCopyRequest{
			SourceBucket: s.opt.bucket, SourceKey: source, DestBucket: s.opt.bucket, DestKey: key,
			IfMatch: ifMatch, IfNoneMatch: ifAbsent, Meta: meta,
		})
		if err == nil {
			return info, "copied in the bucket", nil
		}
		if apiclient.IsPreconditionFailed(err) {
			return nil, "", err
		}
		// The source went meanwhile: reuse or send the content instead.
	}
	if source != "" || (recent && time.Since(rc.seen) < recentFor) {
		info, ok, err := s.client.ReuseFileObject(ctx, s.opt.bucket, key, sha, attrs, ifMatch, ifAbsent)
		if err != nil {
			return nil, "", err
		}
		if ok {
			return info, "reused", nil
		}
	}

	// A link is sent as its target, as the NFS server keeps one.
	var body io.Reader
	ctype := ""
	if l.link {
		body = strings.NewReader(l.target)
	} else {
		f, err := os.Open(local)
		if err != nil {
			return nil, "", err
		}
		defer f.Close()
		body, ctype = f, contentType(local)
		// A large file goes in parts, carrying on from any sent before.
		if info, err := f.Stat(); err == nil && info.Size() >= partedAt {
			out, reused, err := uploadInParts(ctx, s.client, f, info, s.opt.bucket, key, ctype, attrs, ifMatch, ifAbsent)
			how := ""
			if reused > 0 {
				how = "carried on, " + formatBytes(reused) + " already sent"
			}
			return out, how, err
		}
	}
	var info *apiclient.FileObjectInfo
	var err error
	switch {
	case ifMatch != "":
		info, err = s.client.PutFileObjectIfMatch(ctx, s.opt.bucket, key, body, l.size, ctype, attrs, ifMatch)
	case ifAbsent:
		info, err = s.client.PutFileObjectIfAbsent(ctx, s.opt.bucket, key, body, l.size, ctype, attrs)
	default:
		info, err = s.client.PutFileObject(ctx, s.opt.bucket, key, body, l.size, ctype, attrs)
	}
	return info, "", err
}

func (s *syncer) download(ctx context.Context, a action) error {
	r := a.r
	rel := r.rel
	if a.l != nil {
		rel = a.l.rel // keep the local spelling of the name
	}
	local, err := within(s.opt.dir, rel)
	if err != nil {
		return err
	}
	if s.opt.dryRun {
		s.log.Info("would download", "file", rel, "from", remoteName(s.opt.bucket, r.key))
		s.count(func(c *passCounts) { c.downloaded++ })
		return nil
	}
	// Never replace an edit made since the pass looked.
	if !s.unchangedSince(rel, a.l) {
		return nil
	}
	n, err := fetchFile(ctx, s.client, s.opt.bucket, r.key, local)
	if err != nil {
		return err
	}
	s.log.Info("downloaded", "file", rel, "from", remoteName(s.opt.bucket, r.key), "size", formatBytes(n))
	info, err := os.Lstat(local)
	if err != nil {
		return nil
	}
	l := s.newLocal(rel, info)
	if l == nil {
		return nil
	}
	l.sha = r.sha
	s.mu.Lock()
	s.hashes[rel] = hashed{size: l.size, mtime: l.mtime, stamp: l.stamp, sha: r.sha}
	s.hashDirt = true
	s.local[a.id] = l
	s.counts.downloaded++
	s.mu.Unlock()
	s.agree(a.id, l, r)
	return nil
}

func (s *syncer) deleteLocal(a action) error {
	local, err := within(s.opt.dir, a.l.rel)
	if err != nil {
		return err
	}
	if s.opt.dryRun {
		s.log.Info("would delete", "file", a.l.rel)
		s.count(func(c *passCounts) { c.deleted++ })
		return nil
	}
	if !s.unchangedSince(a.l.rel, a.l) {
		return nil
	}
	if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
		return err
	}
	s.log.Info("deleted", "file", a.l.rel)
	s.removeEmptyDirs(filepath.Dir(local))
	s.mu.Lock()
	delete(s.local, a.id)
	delete(s.hashes, a.l.rel)
	if s.opt.mode == modeTwoWay {
		delete(s.base, a.id)
		s.baseDirt = true
	}
	s.counts.deleted++
	s.mu.Unlock()
	return nil
}

// removeEmptyDirs removes directories a deletion left empty, up to the root,
// but not one the bucket keeps as a folder of its own.
func (s *syncer) removeEmptyDirs(dir string) {
	for dir != s.opt.dir && strings.HasPrefix(dir, s.opt.dir+string(filepath.Separator)) {
		id := s.identity(s.rel(dir) + "/")
		s.mu.Lock()
		_, kept := s.markers[id]
		s.mu.Unlock()
		if kept || os.Remove(dir) != nil {
			return
		}
		s.mu.Lock()
		delete(s.dirs, id)
		s.mu.Unlock()
		dir = filepath.Dir(dir)
	}
}

func (s *syncer) deleteRemote(ctx context.Context, a action) error {
	name := remoteName(s.opt.bucket, a.r.key)
	if s.opt.dryRun {
		s.log.Info("would delete", "file", name)
		s.count(func(c *passCounts) { c.deleted++ })
		return nil
	}
	// Only the version the pass looked at is deleted: one changed in the
	// bucket since is left for the next pass, where the change wins.
	var err error
	if a.r.etag != "" {
		err = s.client.DeleteFileObjectIfMatch(ctx, s.opt.bucket, a.r.key, a.r.etag)
	} else {
		err = s.client.DeleteFileObject(ctx, s.opt.bucket, a.r.key)
	}
	if err != nil {
		if apiclient.IsPreconditionFailed(err) {
			s.log.Debug("changed in the bucket meanwhile: looking again", "file", name)
			return nil
		}
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("%s: %w", name, cmdutil.CleanErr(err))
	}
	s.log.Info("deleted", "file", name)
	s.mu.Lock()
	s.noteContent(a.r)
	delete(s.remote, a.id)
	if s.opt.mode == modeTwoWay {
		delete(s.base, a.id)
		s.baseDirt = true
	}
	s.counts.deleted++
	s.mu.Unlock()
	return nil
}

// conflict keeps both versions of a file changed on both sides: the bucket's
// keeps the name, and the local one is renamed beside it, on both sides.
func (s *syncer) conflict(ctx context.Context, a action) error {
	s.count(func(c *passCounts) { c.conflicts++ })
	rel := a.l.rel
	aside := conflictName(rel, time.Now())
	if s.opt.dryRun {
		s.log.Warn("conflict: changed on both sides; the local version would be kept beside it", "file", rel, "local_version", aside)
		return nil
	}
	if !s.unchangedSince(rel, a.l) {
		return nil
	}
	from := filepath.Join(s.opt.dir, filepath.FromSlash(rel))
	to := filepath.Join(s.opt.dir, filepath.FromSlash(aside))
	if err := os.Rename(from, to); err != nil {
		return err
	}
	s.log.Warn("conflict: changed on both sides; the local version is kept beside it", "file", rel, "local_version", aside)

	moved := &localFile{rel: aside, size: a.l.size, mtime: a.l.mtime, mode: a.l.mode, link: a.l.link, target: a.l.target, sha: a.l.sha, stamp: a.l.stamp}
	asideId := s.identity(aside)
	s.mu.Lock()
	delete(s.local, a.id)
	s.local[asideId] = moved
	s.mu.Unlock()
	var errs []error
	if err := s.download(ctx, action{kind: actDownload, id: a.id, r: a.r}); err != nil {
		errs = append(errs, err)
	}
	if err := s.upload(ctx, action{kind: actUpload, id: asideId, l: moved}); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// conflictName is where the local version of a conflicting file is kept:
// name.conflict-<host>-<time>.ext.
func conflictName(rel string, at time.Time) string {
	host, _ := os.Hostname()
	host = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ' ' || r == ':' {
			return '-'
		}
		return r
	}, strings.Split(host, ".")[0])
	if host == "" {
		host = "local"
	}
	dir, file := path.Split(rel)
	ext := path.Ext(file)
	if ext == file {
		ext = "" // a dot file such as .env
	}
	stem := strings.TrimSuffix(file, ext)
	return dir + stem + ".conflict-" + host + "-" + at.Format("20060102-150405") + ext
}

func (s *syncer) count(f func(*passCounts)) {
	s.mu.Lock()
	f(&s.counts)
	s.mu.Unlock()
}

// passed reports a pass that ended to opt.onPass.
func (s *syncer) passed(c passCounts, err error) {
	if s.opt.onPass != nil {
		s.opt.onPass(c, err)
	}
}

// report logs what a pass did, resets the counts and returns them. A single
// sync reports its pass; while watching, each file's change is reported as
// it is made and the passes only at debug level.
func (s *syncer) report() passCounts {
	s.mu.Lock()
	c := s.counts
	s.counts = passCounts{}
	hashed := s.hashedNow
	s.hashedNow = 0
	s.mu.Unlock()
	s.log.Debug("files read for their checksum", "count", hashed)
	msg := "synced"
	if s.opt.dryRun {
		msg = "dry run: nothing changed"
	}
	kv := []any{"uploaded", c.uploaded, "downloaded", c.downloaded, "modes", c.modes, "unchanged", c.unchanged, "deleted", c.deleted}
	switch s.opt.mode {
	case modeUp:
		kv = []any{"uploaded", c.uploaded, "modes", c.modes, "unchanged", c.unchanged, "deleted", c.deleted}
	case modeDown:
		kv = []any{"downloaded", c.downloaded, "modes", c.modes, "unchanged", c.unchanged, "deleted", c.deleted}
	default:
		kv = append(kv, "conflicts", c.conflicts)
	}
	if c.folders > 0 {
		kv = append(kv, "folders", c.folders)
	}
	if s.opt.watching {
		if c.any() {
			s.log.Debug(msg, kv...)
		}
		return c
	}
	s.log.Info(msg, kv...)
	return c
}
