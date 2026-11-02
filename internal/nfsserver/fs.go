package nfsserver

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	billy "github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
	"github.com/paularlott/knot/apiclient"
	nfs "github.com/willscott/go-nfs"
	nfsfile "github.com/willscott/go-nfs/file"
)

// Object metadata holding POSIX attributes, the keys s3fs uses so a bucket
// mounted either way shows the same modes and times; mtime is in the form
// the knot CLI and rclone write.
const (
	metaMode  = "mode"
	metaMtime = "mtime"
	metaUid   = "uid"
	metaGid   = "gid"

	// metaIno is a file's inode number, given when it is created over NFS
	// and kept when it is moved, so a file handle can tell a moved file
	// from another file now at its old path.
	metaIno = "nfs-ino"
)

// started stands in as the time of folders with none of their own.
var started = time.Now()

const (
	defaultFileMode = 0666
	defaultDirMode  = 0777

	// statTTL is how long a file's description is reused. Every NFS call
	// looks up its file handle, so without it each would cost a request to
	// the server; NFS clients cache attributes for longer than this anyway.
	statTTL = 2 * time.Second

	// liveStatTTL is how long while the server tells of changes (see
	// storeFS.Changed), which make a description stale at once.
	liveStatTTL = time.Minute
)

// storeFS presents file storage as one filesystem: the top level holds the
// buckets the user reaches, named as the web UI shows them (the short name
// for their own buckets), and below each bucket its folders and files.
// Folders are key prefixes; an empty folder is a marker object whose key
// ends in "/".
type storeFS struct {
	backend Backend
	staging *staging

	// uid and gid own what has no owner stored, as a local filesystem's
	// files belong to someone: root, for want of one, would leave the
	// user mounting unable to write where the modes say only the owner
	// may. With forceUid or forceGid, they own everything, whatever owner
	// is stored.
	uid, gid           uint32
	forceUid, forceGid bool

	cache *diskCache // nil for none

	// writer names this server in the changes it makes (Config.Writer),
	// so it can pass over its own in change events.
	writer string

	mu        sync.Mutex
	stats     map[string]cachedStat // by statKey
	dirs      map[string]*dirNames  // folders' entries, by statKey of the folder
	buckets   map[string]cachedBucket
	bucketIds map[string]string // bucket names by id, for change events

	// When folders' entries last changed, so a folder's modification time
	// moves as a local one's does: clients judge by it, and by the NFSv4
	// change attribute made from it, whether a listing they hold is still
	// good. By statKey of the folder. Only changes known to be to a
	// folder's entries count: an event that cannot say what changed (on
	// reconnecting, or too much changed to name) moves no folder's time,
	// which would have every folder's time move, and rsync set them all
	// again, for nothing.
	dirChanged map[string]time.Time
	// When a folder's time was last set (touch, rsync): the time set holds
	// until its entries change again.
	dirTimeSet map[string]time.Time
	live       bool // the server tells of changes

	// epochs move on every change to what is kept of a bucket (epochAll,
	// of every bucket), so a lookup or listing answered from before a
	// change, finishing after it, is used but not kept: kept, it could
	// stand in for the change for as long as a description is kept.
	epochs   map[string]uint64
	epochAll uint64
}

// cachedStat is a lookup's result: a description, or nil for nothing there.
type cachedStat struct {
	fi *fileInfo
	at time.Time
}

// dirNames is every name in a folder, as listed or made here: a name not in
// it is known not to be there, without asking the server. Hidden names
// (AppleDouble files) are in it too.
type dirNames struct {
	names map[string]bool
	at    time.Time
}

type cachedBucket struct {
	b  *Bucket
	at time.Time
}

func newStoreFS(backend Backend) *storeFS {
	return &storeFS{backend: backend, stats: map[string]cachedStat{}, dirs: map[string]*dirNames{}, buckets: map[string]cachedBucket{}, bucketIds: map[string]string{},
		dirChanged: map[string]time.Time{}, dirTimeSet: map[string]time.Time{},
		epochs: map[string]uint64{}}
}

var (
	_ billy.Filesystem = (*storeFS)(nil)
	_ billy.Change     = (*storeFS)(nil)
	_ nfs.Committer    = (*storeFS)(nil)
)

func ctx() context.Context { return context.Background() }

// split turns a path into the bucket name and the object key.
func split(name string) (bucket, key string) {
	clean := strings.TrimPrefix(path.Clean("/"+name), "/")
	bucket, key, _ = strings.Cut(clean, "/")
	return bucket, key
}

func statKey(bucket, key string) string { return bucket + "/" + key }

// cached returns what a recent lookup found; ok is false when there is none
// to go by, and fi nil when it found nothing.
func (fs *storeFS) cached(bucket, key string) (fi *fileInfo, ok bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if c, found := fs.stats[statKey(bucket, key)]; found && time.Since(c.at) < fs.ttlLocked() {
		return c.fi, true
	}
	return nil, false
}

// remember records a lookup's result, nil for nothing there.
func (fs *storeFS) remember(bucket, key string, fi *fileInfo) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.rememberLocked(bucket, key, fi)
}

func (fs *storeFS) rememberLocked(bucket, key string, fi *fileInfo) {
	if len(fs.stats) > 100000 {
		fs.stats = map[string]cachedStat{}
	}
	fs.stats[statKey(bucket, key)] = cachedStat{fi: fi, at: time.Now()}
}

// epoch is where a bucket's changes stand, see epochs.
type epoch struct{ bucket, all uint64 }

func (fs *storeFS) epochOf(bucket string) epoch {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return epoch{fs.epochs[bucket], fs.epochAll}
}

// bumpLocked records a change to what is kept of a bucket; fs.mu is held.
func (fs *storeFS) bumpLocked(bucket string) {
	if len(fs.epochs) > 100000 {
		clear(fs.epochs)
		fs.epochAll++
	}
	fs.epochs[bucket]++
}

// rememberAt records a lookup's result made at ep, unless the bucket has
// changed since.
func (fs *storeFS) rememberAt(bucket, key string, fi *fileInfo, ep epoch) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if (epoch{fs.epochs[bucket], fs.epochAll}) == ep {
		fs.rememberLocked(bucket, key, fi)
	}
}

// forget drops what is remembered about a path and the folders above it,
// which a change there can make or unmake; with below, also everything
// under it, for a change to a folder.
func (fs *storeFS) forget(bucket, key string, below bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.bumpLocked(bucket)
	delete(fs.stats, statKey(bucket, key))
	for k := key; k != ""; {
		k = path.Dir(k)
		if k == "." {
			k = ""
		}
		// A folder known to exist still does; one known not to may now.
		if c, ok := fs.stats[statKey(bucket, k)]; ok && (c.fi == nil || !c.fi.IsDir()) {
			delete(fs.stats, statKey(bucket, k))
		}
	}
	if below {
		prefix := statKey(bucket, key)
		if key != "" {
			prefix += "/"
		}
		for k := range fs.stats {
			if strings.HasPrefix(k, prefix) {
				delete(fs.stats, k)
			}
		}
	}
	if key == "" {
		delete(fs.buckets, bucket)
		if below {
			fs.dropDirsLocked(bucket, "", true)
		}
	}
}

// parentOf is the folder holding key, and its name there.
func parentOf(key string) (dir, base string) {
	dir, base = path.Split(key)
	return strings.TrimSuffix(dir, "/"), base
}

// knownAbsent reports whether a recent listing of key's folder shows it is
// not there, so a lookup need not ask.
func (fs *storeFS) knownAbsent(bucket, key string) bool {
	if key == "" {
		return false
	}
	dir, base := parentOf(key)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	d, ok := fs.dirs[statKey(bucket, dir)]
	return ok && time.Since(d.at) < fs.ttlLocked() && !d.names[base]
}

// listed records the names a listing of a folder found, made at ep, unless
// the bucket has changed since.
func (fs *storeFS) listed(bucket, dir string, names map[string]bool, ep epoch) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if (epoch{fs.epochs[bucket], fs.epochAll}) != ep {
		return
	}
	fs.listedLocked(bucket, dir, names)
}

func (fs *storeFS) listedLocked(bucket, dir string, names map[string]bool) {
	if len(fs.dirs) > 10000 {
		fs.dirs = map[string]*dirNames{}
	}
	fs.dirs[statKey(bucket, dir)] = &dirNames{names: names, at: time.Now()}
}

// dirTouchedLocked records that a folder's entries changed now; fs.mu is
// held.
func (fs *storeFS) dirTouchedLocked(bucket, dir string) {
	if len(fs.dirChanged) > 100000 {
		// Too many to keep: forget the oldest half, roughly.
		for k := range fs.dirChanged {
			delete(fs.dirChanged, k)
			if len(fs.dirChanged) <= 50000 {
				break
			}
		}
	}
	fs.dirChanged[statKey(bucket, dir)] = time.Now()
}

// dirMtime is a folder's modification time: when its entries last changed,
// unless its time was set since, or it was not seen to change.
func (fs *storeFS) dirMtime(bucket, dir string, stored time.Time) time.Time {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	k := statKey(bucket, dir)
	changed := fs.dirChanged[k]
	if changed.IsZero() || !changed.After(fs.dirTimeSet[k]) || !changed.After(stored) {
		return stored
	}
	return changed
}

// noteExists records that this server made key: a name now in its folder.
func (fs *storeFS) noteExists(bucket, key string) {
	dir, base := parentOf(key)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if d, ok := fs.dirs[statKey(bucket, dir)]; ok {
		d.names[base] = true
	}
	fs.dirTouchedLocked(bucket, dir)
	fs.bumpLocked(bucket)
}

// noteGone records that this server removed key, and what was below it.
func (fs *storeFS) noteGone(bucket, key string) {
	dir, base := parentOf(key)
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if d, ok := fs.dirs[statKey(bucket, dir)]; ok {
		delete(d.names, base)
	}
	fs.dropDirsLocked(bucket, key, true)
	fs.dirTouchedLocked(bucket, dir)
	fs.bumpLocked(bucket)
}

// dropDirsLocked forgets the names known of a folder and, with below, of the
// folders under it; fs.mu is held.
func (fs *storeFS) dropDirsLocked(bucket, key string, below bool) {
	delete(fs.dirs, statKey(bucket, key))
	if below {
		prefix := statKey(bucket, key)
		if key != "" {
			prefix += "/"
		}
		for k := range fs.dirs {
			if strings.HasPrefix(k, prefix) {
				delete(fs.dirs, k)
			}
		}
	}
}

// bucket describes a bucket, reusing a recent answer.
func (fs *storeFS) bucket(name string) (*Bucket, error) {
	fs.mu.Lock()
	c, ok := fs.buckets[name]
	fs.mu.Unlock()
	if ok && time.Since(c.at) < 5*fs.ttl() {
		return c.b, nil
	}
	b, err := fs.backend.Bucket(ctx(), name)
	if err != nil {
		return nil, err
	}
	fs.mu.Lock()
	fs.buckets[name] = cachedBucket{b: b, at: time.Now()}
	fs.bucketIds[b.Id] = name
	fs.mu.Unlock()
	return b, nil
}

func (fs *storeFS) ttl() time.Duration {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	return fs.ttlLocked()
}

func (fs *storeFS) ttlLocked() time.Duration {
	if fs.live {
		return liveStatTTL
	}
	return statTTL
}

// Live says whether the server is telling of changes, through Changed. A
// description is kept longer while it is, and everything kept is dropped
// when it starts, as changes may have been missed.
func (fs *storeFS) Live(live bool) {
	fs.mu.Lock()
	fs.live = live
	fs.mu.Unlock()
	if live {
		fs.Changed(nil, nil)
	}
}

// Changed drops what is kept of the buckets with these ids, which have
// changed; none means any bucket may have.
//
// Where files names the files that changed in a bucket, only they are
// dropped, and those changed by this server itself (writer) are passed
// over: it changed what it keeps as it made them. A bucket without files
// named changed as a whole.
func (fs *storeFS) Changed(bucketIds []string, files []FileChanged) {
	named := map[string][]FileChanged{}
	for _, f := range files {
		named[f.BucketId] = append(named[f.BucketId], f)
	}

	fs.mu.Lock()
	all := len(bucketIds) == 0
	var whole []string
	for _, id := range bucketIds {
		if _, ok := fs.bucketIds[id]; !ok {
			// A bucket not yet seen by id may still have files kept.
			all = true
			break
		}
		if len(named[id]) == 0 {
			whole = append(whole, fs.bucketIds[id])
		}
	}
	if all {
		clear(fs.stats)
		clear(fs.dirs)
		clear(fs.buckets)
		fs.epochAll++
	}
	names := make(map[string]string, len(named))
	for id := range named {
		names[id] = fs.bucketIds[id]
	}
	fs.mu.Unlock()
	if all {
		fs.backend.Forget("", "")
		return
	}

	for _, name := range whole {
		fs.forget(name, "", true)
		fs.backend.Forget(name, "")
	}
	for id, fl := range named {
		name := names[id]
		if name == "" {
			continue
		}
		for _, f := range fl {
			if fs.writer != "" && f.Writer == fs.writer {
				continue
			}
			fs.forgetChanged(name, f.Key)
			fs.backend.Forget(name, f.Key)
		}
	}
}

// forgetChanged drops what is kept of a file changed elsewhere: the file,
// every folder above it, which a change there can make or unmake, and for a
// folder's marker, what is below it.
func (fs *storeFS) forgetChanged(bucket, key string) {
	folder := strings.HasSuffix(key, "/")
	key = strings.TrimSuffix(key, "/")
	fs.mu.Lock()
	defer fs.mu.Unlock()
	fs.bumpLocked(bucket)
	delete(fs.stats, statKey(bucket, key))
	fs.dropDirsLocked(bucket, key, folder)
	dir, _ := parentOf(key)
	fs.dirTouchedLocked(bucket, dir)
	for k := key; k != ""; {
		k = path.Dir(k)
		if k == "." {
			k = ""
		}
		delete(fs.stats, statKey(bucket, k))
		delete(fs.dirs, statKey(bucket, k))
	}
	if folder {
		prefix := statKey(bucket, key) + "/"
		for k := range fs.stats {
			if strings.HasPrefix(k, prefix) {
				delete(fs.stats, k)
			}
		}
	}
}

func pathErr(op, name string, err error) error {
	if err == nil {
		return nil
	}
	return &os.PathError{Op: op, Path: name, Err: err}
}

// fileInfo describes a bucket, folder or file.
type fileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	mtime time.Time
	id    uint64 // the inode number reported
	ino   uint64 // the stored inode number, 0 when there is none
	sha   string // a file's content, as sha256 hex, when saved
	uid   uint32
	gid   uint32
	// owned says uid and gid were stored; otherwise they are the default
	// owner's, filled in by storeFS.view.
	hasUid, hasGid bool
	// A folder's bucket and key, for its modification time (storeFS.dirMtime).
	dirBucket, dirKey string
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *fileInfo) ModTime() time.Time { return fi.mtime }
func (fi *fileInfo) IsDir() bool        { return fi.mode.IsDir() }

// Sys gives the attributes go-nfs reports. A folder has two links, as POSIX
// has it (its entry and its "."): macOS takes one link to mean no entries and
// skips listing it.
func (fi *fileInfo) Sys() any {
	nlink := uint32(1)
	if fi.IsDir() {
		nlink = 2
	}
	return nfsfile.FileInfo{Nlink: nlink, UID: fi.uid, GID: fi.gid, Fileid: fi.id}
}

// view is the description given out: a copy, under name, owned by the
// default owner unless it has one of its own, or by the owner forced.
func (fs *storeFS) view(fi *fileInfo, name string) *fileInfo {
	c := *fi
	c.name = name
	if c.IsDir() && c.dirBucket != "" {
		c.mtime = fs.dirMtime(c.dirBucket, c.dirKey, c.mtime)
	}
	if !c.hasUid || fs.forceUid {
		c.uid = fs.uid
	}
	if !c.hasGid || fs.forceGid {
		c.gid = fs.gid
	}
	return &c
}

// fileId is a stable inode number for a bucket and key.
func fileId(bucket, key string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(bucket))
	h.Write([]byte{0})
	h.Write([]byte(key))
	return h.Sum64()
}

func dirInfo(name, bucket, key string, mtime time.Time) *fileInfo {
	return &fileInfo{name: name, mode: os.ModeDir | defaultDirMode, mtime: mtime, id: fileId(bucket, key),
		dirBucket: bucket, dirKey: strings.TrimSuffix(key, "/")}
}

// withMeta gives an entry the inode number and ownership stored on it.
func (fi *fileInfo) withMeta(meta map[string]string) *fileInfo {
	if v, err := strconv.ParseUint(meta[metaIno], 16, 64); err == nil && v != 0 {
		fi.ino, fi.id = v, v
	}
	if v, err := strconv.ParseUint(meta[metaUid], 10, 32); err == nil {
		fi.uid, fi.hasUid = uint32(v), true
	}
	if v, err := strconv.ParseUint(meta[metaGid], 10, 32); err == nil {
		fi.gid, fi.hasGid = uint32(v), true
	}
	return fi
}

// withMarker gives a folder the attributes stored on its marker.
func (fi *fileInfo) withMarker(marker *Object) *fileInfo {
	if v, ok := marker.Meta[metaMode]; ok {
		if m, err := strconv.ParseUint(v, 10, 32); err == nil {
			fi.mode = os.ModeDir | os.FileMode(m)&os.ModePerm
		}
	}
	fi.mtime = mtimeFrom(marker.Meta, marker.ModifiedAt)
	return fi.withMeta(marker.Meta)
}

// isAppleDouble reports whether a name is a macOS AppleDouble file, ._name,
// in which macOS keeps the extended attributes of name on a filesystem
// without them. They are kept but left out of listings: listed, rsync
// --delete and the like see files the source does not have, delete them, and
// macOS writes them again.
func isAppleDouble(name string) bool { return strings.HasPrefix(name, "._") }

// appleDouble is the key of the AppleDouble file of key.
func appleDouble(key string) string {
	dir, base := path.Split(key)
	return dir + "._" + base
}

// newIno is a new inode number for metaIno.
func newIno() string {
	var b [8]byte
	for {
		rand.Read(b[:])
		if v := binary.BigEndian.Uint64(b[:]); v != 0 {
			return strconv.FormatUint(v, 16)
		}
	}
}

// modeFrom reads the permission bits stored on a file, s3fs style (the
// decimal st_mode).
func modeFrom(meta map[string]string) os.FileMode {
	if v, ok := meta[metaMode]; ok {
		if m, err := strconv.ParseUint(v, 10, 32); err == nil {
			mode := os.FileMode(m) & os.ModePerm
			if m&syscall.S_IFMT == syscall.S_IFLNK {
				mode |= os.ModeSymlink
			}
			return mode
		}
	}
	return defaultFileMode
}

// stMode is the decimal st_mode stored in metaMode.
func stMode(mode os.FileMode) string {
	typ := uint32(syscall.S_IFREG)
	switch {
	case mode&os.ModeSymlink != 0:
		typ = syscall.S_IFLNK
	case mode&os.ModeDir != 0:
		typ = syscall.S_IFDIR
	}
	return strconv.FormatUint(uint64(typ|uint32(mode&os.ModePerm)), 10)
}

// mtimeFrom reads the modification time stored on a file, falling back to
// when the object was written.
func mtimeFrom(meta map[string]string, fallback time.Time) time.Time {
	if t, ok := apiclient.ParseMtime(meta[metaMtime]); ok {
		return t
	}
	return fallback
}

func objectInfo(name, bucket string, o *Object) *fileInfo {
	return (&fileInfo{
		name:  name,
		size:  o.Size,
		mode:  modeFrom(o.Meta),
		mtime: mtimeFrom(o.Meta, o.ModifiedAt),
		id:    fileId(bucket, o.Key),
		sha:   o.SHA256,
	}).withMeta(o.Meta)
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

func (fs *storeFS) Stat(name string) (os.FileInfo, error) {
	bucket, key := split(name)
	base := path.Base("/" + name)
	if bucket == "" {
		return fs.view(dirInfo("/", "", "", started), "/"), nil
	}
	if key != "" {
		if e := fs.staging.get(bucket, key); e != nil {
			if fi := e.info(base); fi != nil {
				return fs.view(fi, base), nil
			}
		}
	}
	if fi, ok := fs.cached(bucket, key); ok {
		if fi == nil {
			return nil, pathErr("stat", name, os.ErrNotExist)
		}
		return fs.view(fi, base), nil
	}
	if fs.knownAbsent(bucket, key) {
		return nil, pathErr("stat", name, os.ErrNotExist)
	}
	ep := fs.epochOf(bucket)
	fi, err := fs.stat(bucket, key, base)
	if errors.Is(err, os.ErrNotExist) {
		fs.rememberAt(bucket, key, nil, ep)
	}
	if err != nil {
		return nil, pathErr("stat", name, err)
	}
	fs.rememberAt(bucket, key, fi, ep)
	return fs.view(fi, base), nil
}

func (fs *storeFS) stat(bucket, key, base string) (*fileInfo, error) {
	if key == "" {
		b, err := fs.bucket(bucket)
		if err != nil {
			return nil, err
		}
		return dirInfo(base, bucket, "", b.CreatedAt), nil
	}
	st, err := fs.backend.Stat(ctx(), bucket, key)
	if errors.Is(err, os.ErrNotExist) && fs.staging.hasUnder(bucket, key+"/") {
		// A folder holding only files not yet saved.
		return dirInfo(base, bucket, key+"/", started), nil
	}
	if err != nil {
		return nil, err
	}
	if st.File != nil {
		return objectInfo(base, bucket, st.File), nil
	}
	fi := dirInfo(base, bucket, key+"/", started)
	if st.Marker != nil {
		fi.withMarker(st.Marker)
	}
	return fi, nil
}

// isDir reports whether key names a folder: there is a folder marker,
// which it returns, or any file below it, saved or staged. The marker's key
// is the folder's prefix itself, so a listing of the prefix starts with it.
func (fs *storeFS) isDir(bucket, key string) (bool, *Object, error) {
	st, err := fs.backend.Stat(ctx(), bucket, key+"/")
	if errors.Is(err, os.ErrNotExist) {
		return fs.staging.hasUnder(bucket, key+"/"), nil, nil
	}
	if err != nil {
		return false, nil, err
	}
	return st.Folder, st.Marker, nil
}

// readSaved reads part of a saved file whose content is sha: from the disk
// cache when it holds it, else from the server, filling the cache meanwhile
// so the next read need not.
func (fs *storeFS) readSaved(bucket, key, sha string, size int64, b []byte, off int64) (int, error) {
	if f := fs.cache.open(sha); f != nil {
		defer f.Close()
		n, err := f.ReadAt(b, off)
		if err == nil && off+int64(n) >= size {
			err = io.EOF
		}
		return n, err
	}
	if fs.cache.keeps(size) && sha != "" {
		go fs.cache.fill(sha, size, func() (io.ReadCloser, error) {
			r, _, err := fs.backend.Get(ctx(), bucket, key)
			return r, err
		})
	}
	return fs.backend.ReadAt(ctx(), bucket, key, b, off)
}

func (fs *storeFS) Lstat(name string) (os.FileInfo, error) { return fs.Stat(name) }

func (fs *storeFS) ReadDir(name string) ([]os.FileInfo, error) {
	bucket, key := split(name)

	if bucket == "" {
		buckets, err := fs.backend.Buckets(ctx())
		if err != nil {
			return nil, pathErr("readdir", name, err)
		}
		out := make([]os.FileInfo, 0, len(buckets))
		fs.mu.Lock()
		for _, b := range buckets {
			fs.bucketIds[b.Id] = b.Name
		}
		fs.mu.Unlock()
		for _, b := range buckets {
			out = append(out, fs.view(dirInfo(b.Name, b.Name, "", b.CreatedAt), b.Name))
		}
		sortInfos(out)
		return out, nil
	}

	prefix := ""
	if key != "" {
		prefix = key + "/"
	}
	ep := fs.epochOf(bucket)
	entries := map[string]os.FileInfo{}
	names := map[string]bool{} // every name, hidden ones too
	after := ""
	for {
		res, err := fs.backend.List(ctx(), bucket, prefix, "/", after, 1000)
		if err != nil {
			return nil, pathErr("readdir", name, err)
		}
		for _, o := range res.Objects {
			n := o.Key[len(prefix):]
			if n != "" {
				names[n] = true
			}
			if n == "" || isAppleDouble(n) {
				continue // this folder's own marker, or hidden
			}
			if _, ok := entries[n]; !ok {
				fi := objectInfo(n, bucket, o)
				entries[n] = fi
				// The listing describes each file as a lookup would, so
				// the lookups that follow a listing need not ask again.
				fs.rememberAt(bucket, o.Key, fi, ep)
			}
		}
		for _, pfx := range res.Prefixes {
			n := strings.TrimSuffix(pfx[len(prefix):], "/")
			if n == "" {
				continue
			}
			names[n] = true
			// A folder wins over a file of the same name, with the
			// attributes on its marker.
			fi := dirInfo(n, bucket, pfx, started)
			if m := res.Markers[pfx]; m != nil {
				fi.withMarker(m)
				fs.rememberAt(bucket, strings.TrimSuffix(pfx, "/"), fi, ep)
			}
			entries[n] = fi
		}
		if !res.Truncated {
			break
		}
		after = res.Next
	}

	if prefix != "" && len(entries) == 0 {
		// Not listing anything: make sure the folder exists at all.
		if _, err := fs.Stat(name); err != nil {
			return nil, err
		}
	}
	fs.listed(bucket, key, names, ep)

	// Files written but not yet saved, which may be new or changed.
	for n, fi := range fs.staging.list(bucket, prefix) {
		if cur, ok := entries[n]; !ok || !cur.IsDir() {
			entries[n] = fi
		}
	}

	out := make([]os.FileInfo, 0, len(entries))
	for n, fi := range entries {
		out = append(out, fs.view(fi.(*fileInfo), n))
	}
	sortInfos(out)
	return out, nil
}

func sortInfos(infos []os.FileInfo) {
	sort.Slice(infos, func(i, j int) bool { return infos[i].Name() < infos[j].Name() })
}

// ---------------------------------------------------------------------------
// Files
// ---------------------------------------------------------------------------

func (fs *storeFS) Create(name string) (billy.File, error) {
	return fs.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, defaultFileMode)
}

func (fs *storeFS) Open(name string) (billy.File, error) {
	return fs.OpenFile(name, os.O_RDONLY, 0)
}

func (fs *storeFS) OpenFile(name string, flag int, perm os.FileMode) (billy.File, error) {
	bucket, key := split(name)
	if key == "" {
		return nil, pathErr("open", name, syscall.EISDIR)
	}
	writable := flag&(os.O_WRONLY|os.O_RDWR) != 0

	fi, err := fs.Stat(name)
	switch {
	case err == nil && fi.IsDir():
		return nil, pathErr("open", name, syscall.EISDIR)
	case err == nil && flag&os.O_CREATE != 0 && flag&os.O_EXCL != 0:
		return nil, pathErr("open", name, os.ErrExist)
	case err != nil && (flag&os.O_CREATE == 0 || !errors.Is(err, os.ErrNotExist)):
		return nil, err
	}
	exists := err == nil

	if writable || flag&os.O_CREATE != 0 {
		b, err := fs.bucket(bucket)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		if !b.Writable {
			return nil, pathErr("open", name, os.ErrPermission)
		}
	}

	if !exists || (writable && flag&os.O_TRUNC != 0) {
		// A new or truncated file: stage it empty, and it is saved when
		// the client commits it.
		load := loadAttrs
		if !exists {
			load = loadNew
		}
		e, err := fs.staging.open(bucket, key, load)
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		if !exists && perm != 0 {
			e.meta[metaMode] = stMode(perm)
		}
		err = e.truncate(0)
		e.mu.Unlock()
		if err != nil {
			return nil, pathErr("open", name, err)
		}
		if !exists {
			// A new entry in its folder, as the client sees at once.
			dir, _ := parentOf(key)
			fs.mu.Lock()
			fs.dirTouchedLocked(bucket, dir)
			fs.mu.Unlock()
		}
	}

	return &file{fs: fs, name: name, bucket: bucket, key: key}, nil
}

func (fs *storeFS) Remove(name string) error {
	bucket, key := split(name)
	if bucket == "" {
		return pathErr("remove", name, os.ErrPermission)
	}
	defer fs.forget(bucket, key, true)
	if key == "" {
		return pathErr("remove", name, fs.backend.DeleteBucket(ctx(), bucket))
	}

	staged := fs.staging.remove(bucket, key)
	err := fs.backend.Delete(ctx(), bucket, key)
	if err == nil || (staged && errors.Is(err, os.ErrNotExist)) {
		fs.removeAppleDouble(bucket, key)
		fs.noteGone(bucket, key)
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return pathErr("remove", name, err)
	}

	// Not a file: remove an empty folder, which is its marker.
	ok, _, err := fs.isDir(bucket, key)
	if err != nil {
		return pathErr("remove", name, err)
	}
	if !ok {
		return pathErr("remove", name, os.ErrNotExist)
	}
	if fs.staging.hasUnder(bucket, key+"/") {
		return pathErr("remove", name, syscall.ENOTEMPTY)
	}
	// Empty but for AppleDouble files, which are not listed and so cannot
	// be removed first: they go with the folder.
	var hidden []string
	after := ""
	for {
		res, err := fs.backend.List(ctx(), bucket, key+"/", "/", after, 1000)
		if err != nil {
			return pathErr("remove", name, err)
		}
		if len(res.Prefixes) > 0 {
			return pathErr("remove", name, syscall.ENOTEMPTY)
		}
		for _, o := range res.Objects {
			n := o.Key[len(key)+1:]
			switch {
			case n == "":
			case isAppleDouble(n):
				hidden = append(hidden, o.Key)
			default:
				return pathErr("remove", name, syscall.ENOTEMPTY)
			}
		}
		if !res.Truncated {
			break
		}
		after = res.Next
	}
	for _, k := range hidden {
		fs.backend.Delete(ctx(), bucket, k)
	}
	if err := fs.backend.Delete(ctx(), bucket, key+"/"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return pathErr("remove", name, err)
	}
	fs.removeAppleDouble(bucket, key)
	fs.noteGone(bucket, key)
	return nil
}

// removeAppleDouble removes the AppleDouble file of a file or folder just
// removed, which macOS removes itself but other clients leave behind.
func (fs *storeFS) removeAppleDouble(bucket, key string) {
	if isAppleDouble(path.Base(key)) {
		return
	}
	ad := appleDouble(key)
	staged := fs.staging.remove(bucket, ad)
	if !staged && fs.knownAbsent(bucket, ad) {
		return // a listing shows it has none: no need to ask
	}
	err := fs.backend.Delete(ctx(), bucket, ad)
	switch {
	case err == nil, errors.Is(err, os.ErrNotExist):
		fs.forget(bucket, ad, false)
		fs.noteGone(bucket, ad)
	default:
		// Not known whether it is still there: ask again.
		fs.mu.Lock()
		dir, _ := parentOf(ad)
		fs.dropDirsLocked(bucket, dir, false)
		fs.mu.Unlock()
	}
}

func (fs *storeFS) Rename(from, to string) error {
	fb, fk := split(from)
	tb, tk := split(to)
	if fk == "" || tk == "" {
		return pathErr("rename", from, os.ErrPermission) // buckets are renamed in the UI
	}
	if fb != tb {
		return &os.LinkError{Op: "rename", Old: from, New: to, Err: syscall.EXDEV}
	}
	if fk == tk {
		return nil
	}

	src, err := fs.Stat(from)
	if err != nil {
		return err
	}
	if dst, err := fs.Stat(to); err == nil {
		if dst.IsDir() != src.IsDir() {
			if dst.IsDir() {
				return pathErr("rename", to, syscall.EISDIR)
			}
			return pathErr("rename", to, syscall.ENOTDIR)
		}
		if dst.IsDir() {
			// Replacing a folder is allowed when it is empty.
			if err := fs.Remove(to); err != nil {
				return err
			}
		}
	}

	// Moves work on saved files: save what is staged below the source and
	// drop anything staged at the destination, which is being replaced.
	if err := fs.staging.flushUnder(bucketKey(fb, fk), bucketKey(fb, fk+"/")); err != nil {
		return pathErr("rename", from, err)
	}
	fs.staging.remove(tb, tk)

	defer fs.forget(fb, fk, true)
	defer fs.forget(tb, tk, true)
	if err := fs.backend.Move(ctx(), fb, fk, tk); err != nil {
		// What moved, if anything, is not known: ask again of both
		// folders, rather than take the source to be gone.
		fs.mu.Lock()
		fdir, _ := parentOf(fk)
		tdir, _ := parentOf(tk)
		fs.dropDirsLocked(fb, fdir, false)
		fs.dropDirsLocked(tb, tdir, false)
		fs.bumpLocked(fb)
		fs.mu.Unlock()
		return pathErr("rename", from, err)
	}
	fs.noteGone(fb, fk)
	fs.noteExists(tb, tk)
	return nil
}

func (fs *storeFS) MkdirAll(name string, perm os.FileMode) error {
	bucket, key := split(name)
	if bucket == "" {
		return nil
	}
	if fi, err := fs.Stat(name); err == nil {
		if fi.IsDir() {
			return nil
		}
		return pathErr("mkdir", name, os.ErrExist)
	}
	defer fs.forget(bucket, key, false)
	var err error
	if key == "" {
		// A new top level folder is a new bucket of the user's own.
		err = fs.backend.CreateBucket(ctx(), bucket)
	} else {
		_, err = fs.backend.Put(ctx(), bucket, key+"/", bytes.NewReader(nil), 0, "", map[string]string{metaIno: newIno()})
	}
	if err != nil {
		return pathErr("mkdir", name, err)
	}
	// A folder just made holds nothing yet.
	fs.noteExists(bucket, key)
	fs.mu.Lock()
	fs.listedLocked(bucket, key, map[string]bool{})
	fs.mu.Unlock()
	return nil
}

func (fs *storeFS) Join(elem ...string) string { return path.Join(elem...) }

// Commit saves a file's staged writes now: NFS clients commit what they have
// written when the file is closed or synced.
func (fs *storeFS) Commit(name string) error {
	bucket, key := split(name)
	if key == "" {
		return nil
	}
	err := fs.staging.flushUnder(bucketKey(bucket, key), "")
	// Writes dropped since the last commit, refused by the server, are
	// reported now, once; by this commit's error, if it is why.
	dropped := fs.staging.failure(bucket, key, true)
	if err == nil {
		err = dropped
	}
	return pathErr("commit", name, err)
}

func (fs *storeFS) TempFile(dir, prefix string) (billy.File, error) {
	return nil, billy.ErrNotSupported
}

// A symbolic link is stored as s3fs stores one: a file holding the target,
// with the link type in its mode.
func (fs *storeFS) Symlink(target, link string) error {
	bucket, key := split(link)
	if key == "" {
		return pathErr("symlink", link, os.ErrPermission)
	}
	if _, err := fs.Stat(link); err == nil {
		return pathErr("symlink", link, os.ErrExist)
	}
	defer fs.forget(bucket, key, false)
	_, err := fs.backend.Put(ctx(), bucket, key, strings.NewReader(target), int64(len(target)), "",
		map[string]string{metaMode: stMode(os.ModeSymlink | 0777), metaIno: newIno()})
	if err == nil {
		fs.noteExists(bucket, key)
	}
	return pathErr("symlink", link, err)
}

func (fs *storeFS) Readlink(link string) (string, error) {
	fi, err := fs.Lstat(link)
	if err != nil {
		return "", err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return "", pathErr("readlink", link, syscall.EINVAL)
	}
	f, err := fs.Open(link)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b := make([]byte, fi.Size())
	if n, err := f.ReadAt(b, 0); err != nil && !(errors.Is(err, io.EOF) && n == len(b)) {
		return "", err
	}
	return string(b), nil
}

func (fs *storeFS) Chroot(p string) (billy.Filesystem, error) { return chroot.New(fs, p), nil }
func (fs *storeFS) Root() string                              { return "/" }

func (fs *storeFS) Capabilities() billy.Capability {
	return billy.WriteCapability | billy.ReadCapability | billy.ReadAndWriteCapability |
		billy.SeekCapability | billy.TruncateCapability
}

// ---------------------------------------------------------------------------
// Attributes
// ---------------------------------------------------------------------------

func (fs *storeFS) Chmod(name string, mode os.FileMode) error {
	fi, err := fs.Lstat(name)
	if err != nil {
		return err
	}
	switch {
	case fi.Mode()&os.ModeSymlink != 0:
		return nil // a link's own mode is meaningless
	case fi.IsDir():
		mode |= os.ModeDir
	}
	return fs.setMeta(name, map[string]string{metaMode: stMode(mode)})
}

func (fs *storeFS) Chtimes(name string, atime, mtime time.Time) error {
	if err := fs.setMeta(name, map[string]string{metaMtime: apiclient.FormatMtime(mtime)}); err != nil {
		return err
	}
	// A folder's time set holds over the changes to its entries made
	// before it was set (as rsync sets it once the folder is filled).
	if fi, err := fs.Lstat(name); err == nil && fi.IsDir() {
		bucket, key := split(name)
		fs.mu.Lock()
		if len(fs.dirTimeSet) > 100000 {
			clear(fs.dirTimeSet)
		}
		fs.dirTimeSet[statKey(bucket, key)] = time.Now()
		fs.mu.Unlock()
	}
	return nil
}

// Ownership is stored, as s3fs stores it, though not enforced: what a client
// may do is decided by the user the server acts as. A negative id is left as
// it is.
func (fs *storeFS) Lchown(name string, uid, gid int) error {
	kv := map[string]string{}
	if uid >= 0 {
		kv[metaUid] = strconv.Itoa(uid)
	}
	if gid >= 0 {
		kv[metaGid] = strconv.Itoa(gid)
	}
	return fs.setMeta(name, kv)
}

func (fs *storeFS) Chown(name string, uid, gid int) error { return fs.Lchown(name, uid, gid) }

// setMeta records attributes on a file: on its staged copy when it has one,
// else by rewriting its metadata in place (content is not copied). A
// folder's are kept on its marker, made for it if it has none. A bucket
// keeps none.
func (fs *storeFS) setMeta(name string, kv map[string]string) error {
	bucket, key := split(name)
	if key == "" || len(kv) == 0 {
		return nil
	}
	if e := fs.staging.get(bucket, key); e != nil && e.setMeta(kv) {
		return nil
	}
	o, err := fs.backend.Head(ctx(), bucket, key)
	if errors.Is(err, os.ErrNotExist) {
		ok, marker, derr := fs.isDir(bucket, key)
		if derr != nil {
			return pathErr("chattr", name, derr)
		}
		if !ok {
			return pathErr("chattr", name, err)
		}
		defer fs.forget(bucket, key, false)
		if marker == nil {
			meta := map[string]string{metaIno: newIno()}
			for k, v := range kv {
				meta[k] = v
			}
			_, err := fs.backend.Put(ctx(), bucket, key+"/", bytes.NewReader(nil), 0, "", meta)
			return pathErr("chattr", name, err)
		}
		key, o, err = key+"/", marker, nil
	}
	if err != nil {
		return pathErr("chattr", name, err)
	}

	meta := make(map[string]string, len(o.Meta)+len(kv))
	for k, v := range o.Meta {
		meta[k] = v
	}
	same := true
	for k, v := range kv {
		same = same && meta[k] == v
		meta[k] = v
	}
	if same {
		return nil
	}
	defer fs.forget(bucket, strings.TrimSuffix(key, "/"), false)
	return pathErr("chattr", name, fs.backend.SetMeta(ctx(), bucket, key, meta, o.ETag))
}

// ---------------------------------------------------------------------------
// Open files
// ---------------------------------------------------------------------------

// file is a file opened for one NFS call: go-nfs opens, reads or writes,
// and closes the file for every request, so it holds no state of its own.
type file struct {
	fs          *storeFS
	name        string
	bucket, key string
	offset      int64
}

func (f *file) Name() string { return f.name }

func (f *file) ReadAt(b []byte, off int64) (int, error) {
	if e := f.fs.staging.get(f.bucket, f.key); e != nil {
		if n, err, ok := e.readAt(b, off); ok {
			return n, err
		}
	}
	sha, size := "", int64(-1)
	if fi, err := f.fs.Stat(f.name); err == nil {
		sha, size = fi.(*fileInfo).sha, fi.Size()
	}
	n, err := f.fs.readSaved(f.bucket, f.key, sha, size, b, off)
	if err != nil && !errors.Is(err, io.EOF) {
		return n, pathErr("read", f.name, err)
	}
	return n, err
}

func (f *file) Read(b []byte) (int, error) {
	n, err := f.ReadAt(b, f.offset)
	f.offset += int64(n)
	return n, err
}

func (f *file) Write(b []byte) (int, error) {
	e, err := f.fs.staging.open(f.bucket, f.key, loadContent)
	if err != nil {
		return 0, pathErr("write", f.name, err)
	}
	n, err := e.writeAt(b, f.offset)
	e.mu.Unlock()
	f.offset += int64(n)
	return n, err
}

func (f *file) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		f.offset = offset
	case io.SeekCurrent:
		f.offset += offset
	case io.SeekEnd:
		fi, err := f.fs.Stat(f.name)
		if err != nil {
			return 0, err
		}
		f.offset = fi.Size() + offset
	}
	return f.offset, nil
}

func (f *file) Truncate(size int64) error {
	load := loadAttrs
	if size > 0 {
		load = loadContent
	}
	e, err := f.fs.staging.open(f.bucket, f.key, load)
	if err != nil {
		return pathErr("truncate", f.name, err)
	}
	defer e.mu.Unlock()
	return e.truncate(size)
}

func (f *file) Lock() error   { return nil }
func (f *file) Unlock() error { return nil }
func (f *file) Close() error  { return nil }
