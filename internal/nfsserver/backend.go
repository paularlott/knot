package nfsserver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/util/rest"
)

// Backend is the file storage the filesystem serves. Errors are matched with
// errors.Is against os.ErrNotExist, os.ErrExist, os.ErrPermission,
// syscall.ENOTEMPTY, syscall.EDQUOT, syscall.EINVAL and errPrecondition.
//
// Buckets are named as the user sees them: the short name for their own, the
// full name for those shared with them.
type Backend interface {
	Buckets(ctx context.Context) ([]Bucket, error)
	Bucket(ctx context.Context, name string) (*Bucket, error)
	CreateBucket(ctx context.Context, name string) error
	DeleteBucket(ctx context.Context, name string) error

	Head(ctx context.Context, bucket, key string) (*Object, error)
	// Stat says what a path is in one request: a file, or a folder, with
	// its marker (the object keeping an empty folder) when it has one. A
	// path that is neither is os.ErrNotExist.
	Stat(ctx context.Context, bucket, key string) (*Stat, error)
	// List lists a folder; with a delimiter, Listing.Markers holds the
	// markers of the folders rolled up into Prefixes.
	List(ctx context.Context, bucket, prefix, delimiter, after string, max int) (*Listing, error)
	// ReadAt reads part of a file, returning io.EOF with what there is at
	// its end.
	ReadAt(ctx context.Context, bucket, key string, p []byte, off int64) (int, error)
	// Get reads a whole file.
	Get(ctx context.Context, bucket, key string) (io.ReadCloser, *Object, error)
	Put(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string, meta map[string]string) (*Object, error)
	// Patch changes part of a file, sending only the ranges: they are
	// written over it, in order and not overlapping, and it is cut or
	// zero-filled to size. With ifMatch, only if it still has that ETag.
	Patch(ctx context.Context, bucket, key string, ranges []Range, size int64, ifMatch, contentType string, meta map[string]string) (*Object, error)
	Delete(ctx context.Context, bucket, key string) error
	// SetMeta replaces a file's metadata, if it still has the ETag ifMatch.
	SetMeta(ctx context.Context, bucket, key string, meta map[string]string, ifMatch string) error
	// Move renames a file, or a folder and everything under it, replacing
	// what is at the destination.
	Move(ctx context.Context, bucket, from, to string) error
	// Forget drops anything kept of a file changed elsewhere; an empty key
	// means every file in the bucket, and an empty bucket every bucket.
	Forget(bucket, key string)
}

// FileChanged is a file named in a change event, and the writer that
// changed it, when it said.
type FileChanged struct {
	BucketId, Key, Writer string
}

// Range is part of a file to write.
type Range struct {
	Offset, Length int64
	Data           io.Reader
}

// Stat is what a path is.
type Stat struct {
	File   *Object
	Folder bool
	Marker *Object
}

type Bucket struct {
	Name      string
	Id        string
	Writable  bool
	CreatedAt time.Time
}

type Object struct {
	Key         string
	Size        int64
	ETag        string
	SHA256      string
	ContentType string
	ModifiedAt  time.Time
	Meta        map[string]string
}

type Listing struct {
	Objects   []*Object
	Prefixes  []string
	Markers   map[string]*Object // by prefix
	Truncated bool
	Next      string
}

var errPrecondition = errors.New("the file has changed")

// ---------------------------------------------------------------------------
// The files API
// ---------------------------------------------------------------------------

// apiBackend serves file storage through a knot server's files API, as the
// user whose token the client holds.
type apiBackend struct {
	client *apiclient.ApiClient

	mu     sync.Mutex
	blocks map[blockKey]*block
	order  []blockKey // oldest first
}

// NewAPIBackend serves file storage through client.
func NewAPIBackend(client *apiclient.ApiClient) Backend {
	return &apiBackend{client: client, blocks: map[blockKey]*block{}}
}

// Reads are made a block at a time and the blocks kept briefly: NFS clients
// read in pieces of at most a megabyte, and a request to the server for each
// would make reading a file a round trip per megabyte. Blocks are kept no
// longer than file descriptions are (statTTL), and dropped when this server
// changes the file.
const (
	readBlock  = 4 << 20
	readBlocks = 32 // kept at most
)

type blockKey struct {
	bucket, key string
	n           int64
}

type block struct {
	done chan struct{} // closed once fetched
	data []byte
	eof  bool // the block ends the file
	err  error
	at   time.Time
}

// apiErr turns an API error into the errors Backend promises.
func apiErr(err error) error {
	var he *rest.HTTPError
	if err == nil || !errors.As(err, &he) {
		return err
	}
	var e error
	switch he.StatusCode {
	case http.StatusNotFound:
		e = os.ErrNotExist
	case http.StatusForbidden, http.StatusUnauthorized:
		e = os.ErrPermission
	case http.StatusConflict:
		if strings.Contains(he.ServerMessage, "not empty") {
			e = syscall.ENOTEMPTY
		} else {
			e = os.ErrExist
		}
	case http.StatusRequestEntityTooLarge:
		e = syscall.EDQUOT
	case http.StatusPreconditionFailed:
		e = errPrecondition
	case http.StatusBadRequest:
		e = syscall.EINVAL
	default:
		return err
	}
	return fmt.Errorf("%w: %s", e, he.ServerMessage)
}

func bucketFrom(b *apiclient.FileBucketInfo) *Bucket {
	return &Bucket{
		Name:      b.Display,
		Id:        b.Id,
		Writable:  b.Access == "owner" || b.Access == "write",
		CreatedAt: b.CreatedAt,
	}
}

func objectFrom(o *apiclient.FileObjectInfo) *Object {
	return &Object{Key: o.Key, Size: o.Size, ETag: o.ETag, SHA256: o.SHA256, ContentType: o.ContentType, ModifiedAt: o.ModifiedAt, Meta: o.Meta}
}

func (b *apiBackend) Buckets(ctx context.Context) ([]Bucket, error) {
	list, err := b.client.GetFileBuckets(ctx, false)
	if err != nil {
		return nil, apiErr(err)
	}
	out := make([]Bucket, 0, len(list.Buckets))
	for i := range list.Buckets {
		out = append(out, *bucketFrom(&list.Buckets[i]))
	}
	return out, nil
}

func (b *apiBackend) Bucket(ctx context.Context, name string) (*Bucket, error) {
	info, err := b.client.GetFileBucket(ctx, name)
	if err != nil {
		return nil, apiErr(err)
	}
	return bucketFrom(info), nil
}

func (b *apiBackend) CreateBucket(ctx context.Context, name string) error {
	_, err := b.client.CreateFileBucket(ctx, name)
	return apiErr(err)
}

func (b *apiBackend) DeleteBucket(ctx context.Context, name string) error {
	return apiErr(b.client.DeleteFileBucket(ctx, name, false))
}

func (b *apiBackend) Head(ctx context.Context, bucket, key string) (*Object, error) {
	o, err := b.client.HeadFileObject(ctx, bucket, key)
	if err != nil {
		return nil, apiErr(err)
	}
	return objectFrom(o), nil
}

func (b *apiBackend) Stat(ctx context.Context, bucket, key string) (*Stat, error) {
	st, err := b.client.StatFileObject(ctx, bucket, key)
	if err != nil {
		return nil, apiErr(err)
	}
	out := &Stat{Folder: st.Folder}
	if st.File != nil {
		out.File = objectFrom(st.File)
	}
	if st.Marker != nil {
		out.Marker = objectFrom(st.Marker)
	}
	return out, nil
}

func (b *apiBackend) List(ctx context.Context, bucket, prefix, delimiter, after string, max int) (*Listing, error) {
	list := b.client.ListFileObjects
	if delimiter != "" {
		list = b.client.ListFileObjectsMarkers
	}
	res, err := list(ctx, bucket, prefix, delimiter, after, max)
	if err != nil {
		return nil, apiErr(err)
	}
	out := &Listing{Prefixes: res.Prefixes, Truncated: res.IsTruncated, Next: res.Next}
	for i := range res.Objects {
		out.Objects = append(out.Objects, objectFrom(&res.Objects[i]))
	}
	if len(res.Markers) > 0 {
		out.Markers = make(map[string]*Object, len(res.Markers))
		for pfx, m := range res.Markers {
			out.Markers[pfx] = objectFrom(&m)
		}
	}
	return out, nil
}

func (b *apiBackend) ReadAt(ctx context.Context, bucket, key string, p []byte, off int64) (int, error) {
	n := 0
	for n < len(p) {
		at := off + int64(n)
		k := blockKey{bucket, key, at / readBlock}
		blk, err := b.block(ctx, k)
		if err != nil {
			return n, err
		}
		i := at % readBlock
		if i >= int64(len(blk.data)) {
			return n, io.EOF
		}
		c := copy(p[n:], blk.data[i:])
		n += c
		if blk.eof && i+int64(c) == int64(len(blk.data)) {
			return n, io.EOF
		}
		// Reading through a block: fetch the next before it is asked for.
		if !blk.eof && i+int64(c) > readBlock/2 {
			go b.block(context.Background(), blockKey{bucket, key, k.n + 1})
		}
	}
	return n, nil
}

// block returns a block of a file, fetching it unless it is kept; callers
// asking for the same block at once share one fetch.
func (b *apiBackend) block(ctx context.Context, k blockKey) (*block, error) {
	b.mu.Lock()
	if blk, ok := b.blocks[k]; ok {
		select {
		case <-blk.done:
			if blk.err == nil && time.Since(blk.at) < statTTL {
				b.mu.Unlock()
				return blk, nil
			}
		default:
			b.mu.Unlock()
			<-blk.done
			return blk, blk.err
		}
	}
	blk := &block{done: make(chan struct{})}
	b.blocks[k] = blk
	b.order = append(b.order, k)
	for len(b.order) > readBlocks {
		old := b.order[0]
		b.order = b.order[1:]
		if b.blocks[old] != blk {
			delete(b.blocks, old)
		}
	}
	b.mu.Unlock()

	buf := make([]byte, readBlock)
	n, err := b.readRange(ctx, k.bucket, k.key, buf, k.n*readBlock)
	blk.data = buf[:n]
	if errors.Is(err, io.EOF) {
		blk.eof, err = true, nil
	}
	blk.err = err
	blk.at = time.Now()
	close(blk.done)
	return blk, err
}

// forgetBlocks drops the blocks kept of a file, of every file in a bucket
// when key is empty, and of every file when both are.
func (b *apiBackend) forgetBlocks(bucket, key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for k := range b.blocks {
		if (bucket == "" || k.bucket == bucket) && (key == "" || k.key == key) {
			delete(b.blocks, k)
		}
	}
}

func (b *apiBackend) Forget(bucket, key string) {
	b.forgetBlocks(bucket, strings.TrimSuffix(key, "/"))
}

// readRange reads part of a file with one request, io.EOF with what there
// is at its end.
func (b *apiBackend) readRange(ctx context.Context, bucket, key string, p []byte, off int64) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	resp, err := b.client.GetFileObjectRange(ctx, bucket, key, off, int64(len(p)))
	if err != nil {
		return 0, apiErr(err)
	}
	defer resp.Body.Close()

	total := int64(-1)
	if resp.StatusCode == http.StatusPartialContent {
		var start, end int64
		fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &start, &end, &total)
	} else {
		// The whole file: skip to the offset.
		total = resp.ContentLength
		if _, err := io.CopyN(io.Discard, resp.Body, off); err != nil {
			return 0, io.EOF
		}
	}
	n, err := io.ReadFull(resp.Body, p)
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		err = io.EOF
	case err == nil && total >= 0 && off+int64(n) >= total:
		err = io.EOF
	}
	return n, err
}

func (b *apiBackend) Get(ctx context.Context, bucket, key string) (io.ReadCloser, *Object, error) {
	resp, err := b.client.GetFileObject(ctx, bucket, key)
	if err != nil {
		return nil, nil, apiErr(err)
	}
	return resp.Body, objectFrom(apiclient.ObjectInfoFromHeaders(key, resp.Header, resp.ContentLength)), nil
}

func (b *apiBackend) Put(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string, meta map[string]string) (*Object, error) {
	defer b.forgetBlocks(bucket, key)
	o, err := b.client.PutFileObjectMeta(ctx, bucket, key, r, size, contentType, meta)
	if err != nil {
		return nil, apiErr(err)
	}
	return objectFrom(o), nil
}

func (b *apiBackend) Patch(ctx context.Context, bucket, key string, ranges []Range, size int64, ifMatch, contentType string, meta map[string]string) (*Object, error) {
	defer b.forgetBlocks(bucket, key)
	rs := make([]apiclient.FileRange, len(ranges))
	for i, r := range ranges {
		rs[i] = apiclient.FileRange{Offset: r.Offset, Length: r.Length, Data: r.Data}
	}
	o, err := b.client.PatchFileObject(ctx, bucket, key, rs, size, ifMatch, contentType, meta)
	if err != nil {
		return nil, apiErr(err)
	}
	return objectFrom(o), nil
}

func (b *apiBackend) Delete(ctx context.Context, bucket, key string) error {
	defer b.forgetBlocks(bucket, key)
	return apiErr(b.client.DeleteFileObject(ctx, bucket, key))
}

func (b *apiBackend) SetMeta(ctx context.Context, bucket, key string, meta map[string]string, ifMatch string) error {
	if meta == nil {
		meta = map[string]string{}
	}
	_, err := b.client.CopyFileObject(ctx, apiclient.FileCopyRequest{
		SourceBucket: bucket, SourceKey: key, DestBucket: bucket, DestKey: key,
		IfMatch: ifMatch, Meta: meta,
	})
	return apiErr(err)
}

func (b *apiBackend) Move(ctx context.Context, bucket, from, to string) error {
	defer b.forgetBlocks(bucket, "")
	_, err := b.client.MoveFileObjects(ctx, apiclient.FileMoveRequest{Bucket: bucket, From: from, To: to, Overwrite: true})
	return apiErr(err)
}
