package api

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/util/rest"
)

// metaFromRequest reads the user metadata a request carries: the
// X-Knot-Meta-* headers and the file's modification time.
func metaFromRequest(r *http.Request) map[string]string {
	var meta map[string]string
	for k, v := range r.Header {
		if len(v) > 0 && len(k) > len(apiclient.FileMetaHeaderPrefix) && strings.EqualFold(k[:len(apiclient.FileMetaHeaderPrefix)], apiclient.FileMetaHeaderPrefix) {
			if meta == nil {
				meta = map[string]string{}
			}
			meta[strings.ToLower(k[len(apiclient.FileMetaHeaderPrefix):])] = v[0]
		}
	}
	if mtime := r.Header.Get(apiclient.FileMtimeHeader); mtime != "" {
		// Kept as metadata, as S3 clients do, so Last-Modified stays the
		// upload time and rclone sees the same modification time.
		if t, ok := apiclient.ParseMtime(mtime); ok {
			if meta == nil {
				meta = map[string]string{}
			}
			meta["mtime"] = apiclient.FormatMtime(t)
		}
	}
	return meta
}

// HandleStatFileObject says what a path in a bucket is, in one request: a
// file, a folder (with its marker, the object that keeps an empty folder,
// when it has one), or nothing, a 404.
func HandleStatFileObject(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), strings.TrimSuffix(r.PathValue("key"), "/")

	var out apiclient.FileStat
	if key != "" {
		o, err := store.HeadObject(p, bucket, key)
		switch {
		case err == nil:
			info := objectResponse(o)
			out.File = &info
		case !errors.Is(err, filestore.ErrNoSuchKey):
			filesError(w, r, err)
			return
		}
	}
	if out.File == nil {
		prefix := ""
		if key != "" {
			prefix = key + "/"
		}
		res, err := store.ListObjects(p, bucket, prefix, "", "", 1)
		if err != nil {
			filesError(w, r, err)
			return
		}
		// A marker's key is the folder's prefix itself, so it lists first.
		if len(res.Objects) > 0 || key == "" {
			out.Folder = true
			if len(res.Objects) > 0 && res.Objects[0].Key == prefix && prefix != "" {
				info := objectResponse(res.Objects[0])
				out.Marker = &info
			}
		}
	}
	if out.File == nil && !out.Folder {
		filesError(w, r, filestore.ErrNoSuchKey)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, out)
}

var errBadPatch = errors.New("invalid patch")

// HandlePatchFileObject changes part of a file: the body holds byte ranges
// to write over the stored file, and X-Knot-Size the file's new size, so a
// small change to a large file sends only what changed. The new content is
// made on the server from the stored file. If-Match makes the change only if
// the file is still the one the ranges were written against; without it the
// ranges go over whatever the file is when they are applied, made again
// from the newer file when it changes meanwhile (another server's change
// arriving, say), rather than refused.
//
// The body is a sequence of ranges, in order and not overlapping, each an
// 8-byte big-endian offset, an 8-byte big-endian length, then that many
// bytes. Everything past the stored file's end must be sent: the server
// fills nothing in, so a small request cannot make it write a large file.
func HandlePatchFileObject(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	rest.NoDeadlines(w)

	size, err := strconv.ParseInt(r.Header.Get(apiclient.FileSizeHeader), 10, 64)
	if err != nil || size < 0 || size > maxPatchSize {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: apiclient.FileSizeHeader + " must give the new size"})
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	ifMatch := strings.Trim(r.Header.Get("If-Match"), `"`)

	// What can be refused before the body is read is: a change the user may
	// not make, cannot fit, or is to a version no longer there.
	if err := store.CheckPut(p, bucket, key, size); err != nil {
		filesError(w, r, err)
		return
	}
	if ifMatch != "" {
		o, err := store.HeadObject(p, bucket, key)
		if err != nil {
			filesError(w, r, err)
			return
		}
		if o.ETag != ifMatch {
			filesError(w, r, filestore.ErrPrecondition)
			return
		}
	}

	// The ranges are read first, so the file is held below only while the
	// change is made, not while a client sends it, and can be applied again.
	body, err := spoolPatch(store, r.Body, size)
	if err != nil {
		if errors.Is(err, errBadPatch) {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		} else {
			filesError(w, r, err)
		}
		return
	}
	defer body.Close()

	// Patches to one file are made one at a time here, each from the file the
	// last one left, so two at once do not refuse or undo each other.
	defer patchLocks.lock(bucket + "\x00" + key)()

	for attempt := 1; ; attempt++ {
		o, err := patchOnce(r, store, user.Id, p, bucket, key, body, size, ifMatch)
		if errors.Is(err, filestore.ErrPrecondition) && ifMatch == "" && attempt < patchAttempts {
			continue // the file changed as the change was made: make it again
		}
		if errors.Is(err, errBadPatch) {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
			return
		}
		if err != nil {
			filesError(w, r, err)
			return
		}
		rest.WriteResponse(http.StatusOK, w, r, objectResponse(o))
		return
	}
}

// patchAttempts is how many times a patch without If-Match is made, the file
// changing each time as it was.
const patchAttempts = 10

// maxPatchSize is the largest new size a patch may give: one whose body's
// limit (see spoolPatch) can be counted.
const maxPatchSize = (math.MaxInt64 - 16) / 17

// patchOnce makes a patch's change once: from the stored file, which must
// still be the version ifMatch names when it is given, and only if the file
// is still the version it was made from.
func patchOnce(r *http.Request, store *filestore.Store, userId string, p *filestore.Principal, bucket, key string, body *spooledPatch, size int64, ifMatch string) (*filestore.Object, error) {
	base, content, err := store.OpenObject(r.Context(), p, bucket, key)
	if err != nil {
		return nil, err
	}
	defer content.Close()
	if ifMatch != "" && ifMatch != base.ETag {
		return nil, filestore.ErrPrecondition
	}
	ranges, err := body.reader()
	if err != nil {
		return nil, err
	}

	opts := filestore.PutOptions{
		ContentType: r.Header.Get("Content-Type"),
		ModifiedBy:  userId,
		Size:        size,
		Meta:        metaFromRequest(r),
		// The file must still be the one the content was made from.
		IfMatch: base.ETag,
	}
	if opts.ContentType == "" || opts.ContentType == apiclient.FilePatchContentType {
		opts.ContentType = base.ContentType
	}
	if opts.Meta == nil {
		opts.Meta = base.Meta
	}
	pr := &patchReader{base: content, baseLen: base.Size, body: bufio.NewReader(ranges), size: size}
	return store.PutObject(p, bucket, key, pr, opts)
}

// spooledPatch is a patch's body, kept to be read as often as it is applied:
// in memory when small, else in a temporary file of the store's.
type spooledPatch struct {
	data []byte
	f    *os.File
}

// patchInMemory is the largest patch body kept in memory.
const patchInMemory = 1 << 20

// spoolPatch reads a patch's body. It holds at most the new content and a
// header for each range, a range being at least a byte (empty ones are
// skipped as they arrive, so many cannot fill the disk); more is a bad
// patch.
func spoolPatch(store *filestore.Store, r io.Reader, size int64) (*spooledPatch, error) {
	limit := 17*size + 16 // a 16-byte header for every byte, at worst
	buf := make([]byte, 0, min(limit, 64<<10))
	sp := &spooledPatch{}
	var hdr [16]byte
	var total int64
	br := bufio.NewReader(r)
	for {
		if _, err := io.ReadFull(br, hdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			sp.Close()
			return nil, fmt.Errorf("%w: a range header is cut short", errBadPatch)
		}
		n := int64(binary.BigEndian.Uint64(hdr[8:]))
		if n == 0 {
			continue
		}
		if n < 0 || n > size {
			sp.Close()
			return nil, fmt.Errorf("%w: ranges must be in order, not overlap and fall within the new size", errBadPatch)
		}
		total += 16 + n
		if total > limit {
			sp.Close()
			return nil, fmt.Errorf("%w: more ranges than the new size holds", errBadPatch)
		}
		if sp.f == nil && int64(len(buf))+16+n > patchInMemory {
			f, err := store.TempFile("patch-*")
			if err != nil {
				return nil, err
			}
			os.Remove(f.Name()) // gone once closed
			sp.f = f
			if _, err := f.Write(buf); err != nil {
				sp.Close()
				return nil, err
			}
			buf = nil
		}
		if sp.f != nil {
			if _, err := sp.f.Write(hdr[:]); err != nil {
				sp.Close()
				return nil, err
			}
			if _, err := io.CopyN(sp.f, br, n); err != nil {
				sp.Close()
				return nil, fmt.Errorf("%w: a range is cut short", errBadPatch)
			}
			continue
		}
		buf = append(buf, hdr[:]...)
		start := len(buf)
		buf = append(buf, make([]byte, n)...)
		if _, err := io.ReadFull(br, buf[start:]); err != nil {
			return nil, fmt.Errorf("%w: a range is cut short", errBadPatch)
		}
	}
	sp.data = buf
	return sp, nil
}

// reader reads the body from its start.
func (sp *spooledPatch) reader() (io.Reader, error) {
	if sp.f == nil {
		return bytes.NewReader(sp.data), nil
	}
	if _, err := sp.f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return sp.f, nil
}

func (sp *spooledPatch) Close() error {
	if sp.f != nil {
		return sp.f.Close()
	}
	return nil
}

// keyLocks are locks by name, each kept only while held or waited for.
type keyLocks struct {
	mu sync.Mutex
	m  map[string]*keyLock
}

type keyLock struct {
	sync.Mutex
	users int
}

var patchLocks = keyLocks{m: map[string]*keyLock{}}

// lock takes the lock named k, returning its release.
func (l *keyLocks) lock(k string) func() {
	l.mu.Lock()
	kl := l.m[k]
	if kl == nil {
		kl = &keyLock{}
		l.m[k] = kl
	}
	kl.users++
	l.mu.Unlock()
	kl.Lock()
	return func() {
		kl.Unlock()
		l.mu.Lock()
		if kl.users--; kl.users == 0 {
			delete(l.m, k)
		}
		l.mu.Unlock()
	}
}

// patchReader is the new content of a patched file: the stored file, with
// the ranges in body written over it, cut or zero-filled to size. It streams,
// so a patch to a large file needs no more memory than a small one.
type patchReader struct {
	base    io.Reader // the stored file, from its start
	baseLen int64
	body    *bufio.Reader
	size    int64

	pos     int64 // in the new content
	basePos int64 // read from base so far
	next    *patchRange
	inRange int64 // bytes of the current range still to copy from body
	done    bool  // no more ranges in body
}

type patchRange struct{ off, n int64 }

func (pr *patchReader) Read(p []byte) (int, error) {
	for {
		if pr.pos >= pr.size {
			// Every range must have fallen within the file.
			if !pr.done {
				if err := pr.loadNext(); err != nil {
					return 0, err
				}
			}
			if pr.inRange > 0 || pr.next != nil {
				return 0, fmt.Errorf("%w: a range goes past the new size", errBadPatch)
			}
			return 0, io.EOF
		}
		if len(p) == 0 {
			return 0, nil
		}

		if pr.inRange > 0 {
			n := int(min(int64(len(p)), pr.inRange, pr.size-pr.pos))
			n, err := io.ReadFull(pr.body, p[:n])
			if err != nil {
				return 0, fmt.Errorf("%w: a range is cut short", errBadPatch)
			}
			pr.pos += int64(n)
			pr.inRange -= int64(n)
			return n, nil
		}

		if pr.next == nil && !pr.done {
			if err := pr.loadNext(); err != nil {
				return 0, err
			}
		}
		if pr.next != nil && pr.next.off == pr.pos {
			pr.inRange, pr.next = pr.next.n, nil
			continue
		}

		// Up to the next range, the stored file, then zeros past its end.
		limit := pr.size - pr.pos
		if pr.next != nil {
			limit = pr.next.off - pr.pos
		}
		if err := pr.seekBase(); err != nil {
			return 0, err
		}
		if pr.pos < pr.baseLen {
			n := int(min(int64(len(p)), limit, pr.baseLen-pr.pos))
			n, err := io.ReadFull(pr.base, p[:n])
			pr.basePos += int64(n)
			pr.pos += int64(n)
			if err != nil {
				return n, err
			}
			return n, nil
		}
		// Past the stored file, with no range here: not filled in.
		return 0, fmt.Errorf("%w: bytes past the end of the stored file must be sent", errBadPatch)
	}
}

// seekBase moves base to the position in the new content, skipping what
// ranges wrote over.
func (pr *patchReader) seekBase() error {
	if pr.basePos >= pr.pos || pr.basePos >= pr.baseLen {
		return nil
	}
	skip := min(pr.pos, pr.baseLen) - pr.basePos
	if _, err := io.CopyN(io.Discard, pr.base, skip); err != nil {
		return err
	}
	pr.basePos += skip
	return nil
}

// loadNext reads the next range's header, if there is one.
func (pr *patchReader) loadNext() error {
	var hdr [16]byte
	for {
		if _, err := io.ReadFull(pr.body, hdr[:]); err != nil {
			if errors.Is(err, io.EOF) {
				pr.done = true
				return nil
			}
			return fmt.Errorf("%w: a range header is cut short", errBadPatch)
		}
		off, n := int64(binary.BigEndian.Uint64(hdr[:8])), int64(binary.BigEndian.Uint64(hdr[8:]))
		if off < pr.pos || n < 0 || off > pr.size || n > pr.size-off {
			return fmt.Errorf("%w: ranges must be in order, not overlap and fall within the new size", errBadPatch)
		}
		if n == 0 {
			continue
		}
		pr.next = &patchRange{off: off, n: n}
		return nil
	}
}
