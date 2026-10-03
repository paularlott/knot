package filestore

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// blobStore keeps content addressed by sha256 under dir/ab/cd/<sha>.
type blobStore struct {
	dir    string
	tmpDir string
}

func newBlobStore(dir, tmpDir string) (*blobStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &blobStore{dir: dir, tmpDir: tmpDir}, nil
}

func validSHA(sha string) bool {
	if len(sha) != 64 {
		return false
	}
	for i := 0; i < len(sha); i++ {
		c := sha[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (bs *blobStore) path(sha string) string {
	return filepath.Join(bs.dir, sha[0:2], sha[2:4], sha)
}

func (bs *blobStore) has(sha string) bool {
	if !validSHA(sha) {
		return false
	}
	_, err := os.Stat(bs.path(sha))
	return err == nil
}

func (bs *blobStore) open(sha string) (*os.File, error) {
	if !validSHA(sha) {
		return nil, os.ErrNotExist
	}
	return os.Open(bs.path(sha))
}

func (bs *blobStore) remove(sha string) {
	if validSHA(sha) {
		os.Remove(bs.path(sha))
	}
}

// install moves a fully written temporary file into place as sha.
func (bs *blobStore) install(tmpPath, sha string) error {
	dst := bs.path(sha)
	if _, err := os.Stat(dst); err == nil {
		// Identical content is already stored.
		os.Remove(tmpPath)
		return nil
	}
	// The sweep may remove an empty directory between creating it and the
	// rename, so recreate it and retry.
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return err
		}
		if err = os.Rename(tmpPath, dst); err == nil || !os.IsNotExist(err) {
			return err
		}
	}
	return err
}

func (bs *blobStore) walk(fn func(sha string)) {
	filepath.WalkDir(bs.dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && validSHA(d.Name()) {
			fn(d.Name())
		}
		return nil
	})
}

// removeEmptyDirs removes the fan-out directories left empty by deletes.
func (bs *blobStore) removeEmptyDirs() {
	top, err := os.ReadDir(bs.dir)
	if err != nil {
		return
	}
	for _, t := range top {
		if !t.IsDir() {
			continue
		}
		first := filepath.Join(bs.dir, t.Name())
		subs, _ := os.ReadDir(first)
		for _, sd := range subs {
			if sd.IsDir() {
				// Remove fails harmlessly when the directory isn't empty.
				os.Remove(filepath.Join(first, sd.Name()))
			}
		}
		os.Remove(first)
	}
}

// tempWriter streams content into a temporary file while hashing it.
type tempWriter struct {
	f      *os.File
	sha    hash.Hash
	md5    hash.Hash
	size   int64
	closed bool
}

func (bs *blobStore) newTemp() (*tempWriter, error) {
	f, err := os.CreateTemp(bs.tmpDir, "upload-*")
	if err != nil {
		return nil, err
	}
	return &tempWriter{f: f, sha: sha256.New(), md5: md5.New()}, nil
}

func (t *tempWriter) Write(p []byte) (int, error) {
	n, err := t.f.Write(p)
	t.sha.Write(p[:n])
	t.md5.Write(p[:n])
	t.size += int64(n)
	return n, err
}

// ReadFrom copies r into the file, returning the bytes copied.
func (t *tempWriter) ReadFrom(r io.Reader) (int64, error) {
	buf := make([]byte, 256*1024)
	return io.CopyBuffer(struct{ io.Writer }{t}, r, buf)
}

func (t *tempWriter) SHA256() string { return hex.EncodeToString(t.sha.Sum(nil)) }
func (t *tempWriter) MD5() []byte    { return t.md5.Sum(nil) }
func (t *tempWriter) Path() string   { return t.f.Name() }

// finish flushes and closes the file, leaving it in place.
func (t *tempWriter) finish() error {
	if t.closed {
		return nil
	}
	t.closed = true
	if err := t.f.Sync(); err != nil {
		t.f.Close()
		return err
	}
	return t.f.Close()
}

// writeTemp streams r into a finished temporary file, checking its size
// (when size >= 0) and any expected checksums. With max >= 0 a body larger
// than max is refused as soon as it passes it, so an upload of unknown size
// cannot fill the disk beyond the owner's quota. The caller installs or
// discards the file.
func (bs *blobStore) writeTemp(r io.Reader, size, max int64, sha string, md5sum []byte) (*tempWriter, error) {
	tw, err := bs.newTemp()
	if err != nil {
		return nil, err
	}
	if max >= 0 {
		r = io.LimitReader(r, max+1)
	}
	if _, err := tw.ReadFrom(r); err != nil {
		tw.discard()
		return nil, err
	}
	if max >= 0 && tw.size > max {
		tw.discard()
		return nil, ErrQuotaExceeded
	}
	if size >= 0 && size != tw.size {
		tw.discard()
		return nil, io.ErrUnexpectedEOF
	}
	if (sha != "" && !strings.EqualFold(sha, tw.SHA256())) || (len(md5sum) > 0 && !bytes.Equal(md5sum, tw.MD5())) {
		tw.discard()
		return nil, ErrContentMismatch
	}
	if err := tw.finish(); err != nil {
		tw.discard()
		return nil, err
	}
	return tw, nil
}

// discard closes and removes the file.
func (t *tempWriter) discard() {
	if !t.closed {
		t.closed = true
		t.f.Close()
	}
	os.Remove(t.f.Name())
}
