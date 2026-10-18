package nfsserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/paularlott/knot/internal/filestore"
)

// storeBackend serves a filestore.Store directly, so the filesystem can be
// tested without a knot server; it maps errors as the files API does.
type storeBackend struct {
	store *filestore.Store
	p     *filestore.Principal
}

func storeErr(err error) error {
	var e error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, filestore.ErrNoSuchBucket), errors.Is(err, filestore.ErrNoSuchKey):
		e = os.ErrNotExist
	case errors.Is(err, filestore.ErrAccessDenied), errors.Is(err, filestore.ErrCannotOwn):
		e = os.ErrPermission
	case errors.Is(err, filestore.ErrBucketNotEmpty):
		e = syscall.ENOTEMPTY
	case errors.Is(err, filestore.ErrBucketExists), errors.Is(err, filestore.ErrDestinationExists):
		e = os.ErrExist
	case errors.Is(err, filestore.ErrQuotaExceeded):
		e = syscall.EDQUOT
	case errors.Is(err, filestore.ErrPrecondition):
		e = errPrecondition
	case errors.Is(err, filestore.ErrInvalidKey), errors.Is(err, filestore.ErrInvalidName), errors.Is(err, filestore.ErrInvalidShortName):
		e = syscall.EINVAL
	default:
		return err
	}
	return fmt.Errorf("%w: %v", e, err)
}

func (b *storeBackend) bucket(name string) string { return filestore.ResolveName(b.p, name) }

func (b *storeBackend) Buckets(context.Context) ([]Bucket, error) {
	var out []Bucket
	for _, a := range b.store.AccessibleBuckets(b.p) {
		info, err := b.store.GetBucket(b.p, a.Name)
		if err != nil {
			continue
		}
		out = append(out, Bucket{Name: filestore.DisplayName(b.p, a.Name), Id: info.Id, Writable: info.Access >= filestore.AccessWrite, CreatedAt: info.CreatedAt})
	}
	return out, nil
}

func (b *storeBackend) Bucket(_ context.Context, name string) (*Bucket, error) {
	info, err := b.store.GetBucket(b.p, b.bucket(name))
	if err != nil {
		return nil, storeErr(err)
	}
	return &Bucket{Name: name, Id: info.Id, Writable: info.Access >= filestore.AccessWrite, CreatedAt: info.CreatedAt}, nil
}

func (b *storeBackend) CreateBucket(_ context.Context, name string) error {
	full, err := filestore.NewBucketName(b.p, name)
	if err != nil {
		return storeErr(err)
	}
	_, err = b.store.CreateBucket(b.p, full)
	return storeErr(err)
}

func (b *storeBackend) DeleteBucket(_ context.Context, name string) error {
	return storeErr(b.store.DeleteBucket(b.p, b.bucket(name), false))
}

func toObject(o *filestore.Object) *Object {
	return &Object{Key: o.Key, Size: o.Size, ETag: o.ETag, SHA256: o.SHA256, ContentType: o.ContentType, ModifiedAt: o.ModifiedAt, Meta: o.Meta}
}

// patches counts the patches made, so tests can tell a file was changed in
// place rather than sent whole.
var patches atomic.Int32

func (b *storeBackend) Stat(_ context.Context, bucket, key string) (*Stat, error) {
	key = strings.TrimSuffix(key, "/")
	if key != "" {
		if o, err := b.store.HeadObject(b.p, b.bucket(bucket), key); err == nil {
			return &Stat{File: toObject(o)}, nil
		} else if !errors.Is(err, filestore.ErrNoSuchKey) {
			return nil, storeErr(err)
		}
	}
	prefix := ""
	if key != "" {
		prefix = key + "/"
	}
	res, err := b.store.ListObjects(b.p, b.bucket(bucket), prefix, "", "", 1)
	if err != nil {
		return nil, storeErr(err)
	}
	if len(res.Objects) == 0 && key != "" {
		return nil, os.ErrNotExist
	}
	st := &Stat{Folder: true}
	if len(res.Objects) > 0 && res.Objects[0].Key == prefix && prefix != "" {
		st.Marker = toObject(res.Objects[0])
	}
	return st, nil
}

func (b *storeBackend) Patch(ctx context.Context, bucket, key string, ranges []Range, size int64, ifMatch, contentType string, meta map[string]string) (*Object, error) {
	patches.Add(1)
	o, c, err := b.store.OpenObject(ctx, b.p, b.bucket(bucket), key)
	if err != nil {
		return nil, storeErr(err)
	}
	base, err := io.ReadAll(c)
	c.Close()
	if err != nil {
		return nil, err
	}
	if ifMatch != "" && ifMatch != o.ETag {
		return nil, errPrecondition
	}
	out := make([]byte, size)
	copy(out, base)
	for _, r := range ranges {
		if _, err := io.ReadFull(r.Data, out[r.Offset:r.Offset+r.Length]); err != nil {
			return nil, err
		}
	}
	if contentType == "" {
		contentType = o.ContentType
	}
	return b.Put(ctx, bucket, key, bytes.NewReader(out), size, contentType, meta)
}

func (b *storeBackend) Forget(string, string) {}

func (b *storeBackend) Head(_ context.Context, bucket, key string) (*Object, error) {
	o, err := b.store.HeadObject(b.p, b.bucket(bucket), key)
	if err != nil {
		return nil, storeErr(err)
	}
	return toObject(o), nil
}

func (b *storeBackend) List(_ context.Context, bucket, prefix, delimiter, after string, max int) (*Listing, error) {
	res, err := b.store.ListObjects(b.p, b.bucket(bucket), prefix, delimiter, after, max)
	if err != nil {
		return nil, storeErr(err)
	}
	out := &Listing{Prefixes: res.Prefixes, Truncated: res.IsTruncated, Next: res.Next}
	for _, o := range res.Objects {
		out.Objects = append(out.Objects, toObject(o))
	}
	for _, pfx := range res.Prefixes {
		if m, err := b.store.HeadObject(b.p, b.bucket(bucket), pfx); err == nil {
			if out.Markers == nil {
				out.Markers = map[string]*Object{}
			}
			out.Markers[pfx] = toObject(m)
		}
	}
	return out, nil
}

func (b *storeBackend) ReadAt(ctx context.Context, bucket, key string, p []byte, off int64) (int, error) {
	_, c, err := b.store.OpenObject(ctx, b.p, b.bucket(bucket), key)
	if err != nil {
		return 0, storeErr(err)
	}
	defer c.Close()
	if _, err := c.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}
	n, err := io.ReadFull(c, p)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	return n, err
}

func (b *storeBackend) Get(ctx context.Context, bucket, key string) (io.ReadCloser, *Object, error) {
	o, c, err := b.store.OpenObject(ctx, b.p, b.bucket(bucket), key)
	if err != nil {
		return nil, nil, storeErr(err)
	}
	return c, toObject(o), nil
}

func (b *storeBackend) Put(_ context.Context, bucket, key string, r io.Reader, size int64, contentType string, meta map[string]string) (*Object, error) {
	o, err := b.store.PutObject(b.p, b.bucket(bucket), key, r, filestore.PutOptions{Size: size, ContentType: contentType, Meta: meta, ModifiedBy: b.p.UserId})
	if err != nil {
		return nil, storeErr(err)
	}
	return toObject(o), nil
}

func (b *storeBackend) Delete(_ context.Context, bucket, key string) error {
	return storeErr(b.store.DeleteObject(b.p, b.bucket(bucket), key))
}

func (b *storeBackend) SetMeta(_ context.Context, bucket, key string, meta map[string]string, ifMatch string) error {
	full := b.bucket(bucket)
	_, err := b.store.CopyObjectIf(b.p, full, key, full, key, &filestore.PutOptions{Meta: meta, ModifiedBy: b.p.UserId}, filestore.PutOptions{IfMatch: ifMatch})
	return storeErr(err)
}

func (b *storeBackend) Move(_ context.Context, bucket, from, to string) error {
	_, err := b.store.MoveObjects(b.p, b.bucket(bucket), from, to, true)
	return storeErr(err)
}

func putOpts(size int64) filestore.PutOptions { return filestore.PutOptions{Size: size} }
