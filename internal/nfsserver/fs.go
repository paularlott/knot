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
)

// storeFS presents file storage as one filesystem: the top level holds the
// buckets the user reaches, named as the web UI shows them (the short name
// for their own buckets), and below each bucket its folders and files.
// Folders are key prefixes; an empty folder is a marker object whose key
// ends in "/".
type storeFS struct {
	backend Backend
	staging *staging

	mu      sync.Mutex
	stats   map[string]cachedStat // by statKey
	buckets map[string]cachedBucket
}

// cachedStat is a lookup's result: a description, or nil for nothing there.
type cachedStat struct {
	fi *fileInfo
	at time.Time
}

type cachedBucket struct {
	b  *Bucket
	at time.Time
}

func newStoreFS(backend Backend) *storeFS {
	return &storeFS{backend: backend, stats: map[string]cachedStat{}, buckets: map[string]cachedBucket{}}
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
	if c, found := fs.stats[statKey(bucket, key)]; found && time.Since(c.at) < statTTL {
		return c.fi, true
	}
	return nil, false
}

// remember records a lookup's result, nil for nothing there.
func (fs *storeFS) remember(bucket, key string, fi *fileInfo) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
	if len(fs.stats) > 100000 {
		fs.stats = map[string]cachedStat{}
	}
	fs.stats[statKey(bucket, key)] = cachedStat{fi: fi, at: time.Now()}
}

// forget drops what is remembered about a path and the folders above it,
// which a change there can make or unmake; with below, also everything
// under it, for a change to a folder.
func (fs *storeFS) forget(bucket, key string, below bool) {
	fs.mu.Lock()
	defer fs.mu.Unlock()
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
	}
}

// bucket describes a bucket, reusing a recent answer.
func (fs *storeFS) bucket(name string) (*Bucket, error) {
	fs.mu.Lock()
	c, ok := fs.buckets[name]
	fs.mu.Unlock()
	if ok && time.Since(c.at) < 5*statTTL {
		return c.b, nil
	}
	b, err := fs.backend.Bucket(ctx(), name)
	if err != nil {
		return nil, err
	}
	fs.mu.Lock()
	fs.buckets[name] = cachedBucket{b: b, at: time.Now()}
	fs.mu.Unlock()
	return b, nil
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
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() os.FileMode  { return fi.mode }
func (fi *fileInfo) ModTime() time.Time { return fi.mtime }
func (fi *fileInfo) IsDir() bool        { return fi.mode.IsDir() }
func (fi *fileInfo) Sys() any {
	return nfsfile.FileInfo{Nlink: 1, Fileid: fi.id}
}

// named is the description under another name.
func (fi *fileInfo) named(name string) *fileInfo {
	c := *fi
	c.name = name
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
	return &fileInfo{name: name, mode: os.ModeDir | defaultDirMode, mtime: mtime, id: fileId(bucket, key)}
}

// withIno gives an entry its stored inode number, if it has one.
func (fi *fileInfo) withIno(meta map[string]string) *fileInfo {
	if v, err := strconv.ParseUint(meta[metaIno], 16, 64); err == nil && v != 0 {
		fi.ino, fi.id = v, v
	}
	return fi
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
	if mode&os.ModeSymlink != 0 {
		typ = syscall.S_IFLNK
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
	}).withIno(o.Meta)
}

// ---------------------------------------------------------------------------
// Lookups
// ---------------------------------------------------------------------------

func (fs *storeFS) Stat(name string) (os.FileInfo, error) {
	bucket, key := split(name)
	base := path.Base("/" + name)
	if bucket == "" {
		return dirInfo("/", "", "", started), nil
	}
	if key != "" {
		if e := fs.staging.get(bucket, key); e != nil {
			if fi := e.info(base); fi != nil {
				return fi, nil
			}
		}
	}
	if fi, ok := fs.cached(bucket, key); ok {
		if fi == nil {
			return nil, pathErr("stat", name, os.ErrNotExist)
		}
		return fi.named(base), nil
	}
	fi, err := fs.stat(bucket, key, base)
	if errors.Is(err, os.ErrNotExist) {
		fs.remember(bucket, key, nil)
	}
	if err != nil {
		return nil, pathErr("stat", name, err)
	}
	fs.remember(bucket, key, fi)
	return fi, nil
}

func (fs *storeFS) stat(bucket, key, base string) (*fileInfo, error) {
	if key == "" {
		b, err := fs.bucket(bucket)
		if err != nil {
			return nil, err
		}
		return dirInfo(base, bucket, "", b.CreatedAt), nil
	}
	// A file or a folder: ask both at once, as each is a round trip to
	// the server.
	type dirResult struct {
		ok     bool
		marker *Object
		err    error
	}
	dirCh := make(chan dirResult, 1)
	go func() {
		ok, marker, err := fs.isDir(bucket, key)
		dirCh <- dirResult{ok, marker, err}
	}()
	o, err := fs.backend.Head(ctx(), bucket, key)
	if err == nil {
		return objectInfo(base, bucket, o), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	d := <-dirCh
	ok, marker, err := d.ok, d.marker, d.err
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, os.ErrNotExist
	}
	fi := dirInfo(base, bucket, key+"/", started)
	if marker != nil {
		fi.mtime = marker.ModifiedAt
		fi.withIno(marker.Meta)
	}
	return fi, nil
}

// isDir reports whether key names a folder: there is a folder marker,
// which it returns, or any file below it, saved or staged. The marker's key
// is the folder's prefix itself, so a listing of the prefix starts with it.
func (fs *storeFS) isDir(bucket, key string) (bool, *Object, error) {
	res, err := fs.backend.List(ctx(), bucket, key+"/", "", "", 1)
	if err != nil {
		return false, nil, err
	}
	if len(res.Objects) > 0 {
		if o := res.Objects[0]; o.Key == key+"/" {
			return true, o, nil
		}
		return true, nil, nil
	}
	return fs.staging.hasUnder(bucket, key+"/"), nil, nil
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
		for _, b := range buckets {
			out = append(out, dirInfo(b.Name, b.Name, "", b.CreatedAt))
		}
		sortInfos(out)
		return out, nil
	}

	prefix := ""
	if key != "" {
		prefix = key + "/"
	}
	entries := map[string]os.FileInfo{}
	after := ""
	for {
		res, err := fs.backend.List(ctx(), bucket, prefix, "/", after, 1000)
		if err != nil {
			return nil, pathErr("readdir", name, err)
		}
		for _, o := range res.Objects {
			n := o.Key[len(prefix):]
			if n == "" {
				continue // this folder's own marker
			}
			if _, ok := entries[n]; !ok {
				fi := objectInfo(n, bucket, o)
				entries[n] = fi
				// The listing describes each file as a lookup would, so
				// the lookups that follow a listing need not ask again.
				fs.remember(bucket, o.Key, fi)
			}
		}
		for _, pfx := range res.Prefixes {
			n := strings.TrimSuffix(pfx[len(prefix):], "/")
			if n == "" {
				continue
			}
			// A folder wins over a file of the same name.
			entries[n] = dirInfo(n, bucket, pfx, started)
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

	// Files written but not yet saved, which may be new or changed.
	for n, fi := range fs.staging.list(bucket, prefix) {
		if cur, ok := entries[n]; !ok || !cur.IsDir() {
			entries[n] = fi
		}
	}

	out := make([]os.FileInfo, 0, len(entries))
	for _, fi := range entries {
		out = append(out, fi)
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
	res, err := fs.backend.List(ctx(), bucket, key+"/", "", "", 2)
	if err != nil {
		return pathErr("remove", name, err)
	}
	for _, o := range res.Objects {
		if o.Key != key+"/" {
			return pathErr("remove", name, syscall.ENOTEMPTY)
		}
	}
	if err := fs.backend.Delete(ctx(), bucket, key+"/"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return pathErr("remove", name, err)
	}
	return nil
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
	return pathErr("rename", from, fs.backend.Move(ctx(), fb, fk, tk))
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
	if key == "" {
		// A new top level folder is a new bucket of the user's own.
		return pathErr("mkdir", name, fs.backend.CreateBucket(ctx(), bucket))
	}
	err := fs.backend.Put(ctx(), bucket, key+"/", bytes.NewReader(nil), 0, "", map[string]string{metaIno: newIno()})
	return pathErr("mkdir", name, err)
}

func (fs *storeFS) Join(elem ...string) string { return path.Join(elem...) }

// Commit saves a file's staged writes now: NFS clients commit what they have
// written when the file is closed or synced.
func (fs *storeFS) Commit(name string) error {
	bucket, key := split(name)
	if key == "" {
		return nil
	}
	return pathErr("commit", name, fs.staging.flushUnder(bucketKey(bucket, key), ""))
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
	err := fs.backend.Put(ctx(), bucket, key, strings.NewReader(target), int64(len(target)), "",
		map[string]string{metaMode: stMode(os.ModeSymlink | 0777), metaIno: newIno()})
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
	if fi, err := fs.Lstat(name); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		return nil // a link's own mode is meaningless
	}
	return fs.setMeta(name, metaMode, stMode(mode))
}

func (fs *storeFS) Chtimes(name string, atime, mtime time.Time) error {
	return fs.setMeta(name, metaMtime, apiclient.FormatMtime(mtime))
}

// Ownership is not stored: every file belongs to whoever mounts.
func (fs *storeFS) Lchown(name string, uid, gid int) error { return nil }
func (fs *storeFS) Chown(name string, uid, gid int) error  { return nil }

// setMeta records an attribute on a file: on its staged copy when it has
// one, else by rewriting its metadata in place (content is not copied).
// Folders keep no attributes.
func (fs *storeFS) setMeta(name, k, v string) error {
	bucket, key := split(name)
	if key == "" {
		return nil
	}
	if e := fs.staging.get(bucket, key); e != nil && e.setMeta(k, v) {
		return nil
	}
	o, err := fs.backend.Head(ctx(), bucket, key)
	if errors.Is(err, os.ErrNotExist) {
		if ok, _, _ := fs.isDir(bucket, key); ok {
			return nil
		}
	}
	if err != nil {
		return pathErr("chattr", name, err)
	}
	if o.Meta[k] == v {
		return nil
	}
	meta := make(map[string]string, len(o.Meta)+1)
	for mk, mv := range o.Meta {
		meta[mk] = mv
	}
	meta[k] = v
	defer fs.forget(bucket, key, false)
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
	n, err := f.fs.backend.ReadAt(ctx(), f.bucket, f.key, b, off)
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
