package filestore

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const multipartMaxAge = 7 * 24 * time.Hour

var (
	ErrNoSuchUpload = errors.New("multipart upload not found")
	ErrInvalidPart  = errors.New("invalid multipart part")
	ErrInvalidRange = errors.New("the range is not valid for the source object")
)

// MultipartUpload is an upload in progress. Parts are held on the server
// that received them until the upload completes.
type MultipartUpload struct {
	Id          string            `json:"id"`
	Bucket      string            `json:"bucket"`
	Key         string            `json:"key"`
	ContentType string            `json:"content_type"`
	Meta        map[string]string `json:"meta"`
	Created     time.Time         `json:"created"`
}

// PartInfo describes an uploaded part.
type PartInfo struct {
	Number   int       `json:"number"`
	ETag     string    `json:"etag"`
	Size     int64     `json:"size"`
	Modified time.Time `json:"modified"`
}

// CompletedPart names a part to include when completing an upload.
type CompletedPart struct {
	Number int
	ETag   string
}

func (s *Store) uploadDir(id string) string {
	return filepath.Join(s.dir, "multipart", id)
}

func validUploadId(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// CreateMultipart starts a multipart upload.
func (s *Store) CreateMultipart(p *Principal, bucket, key string, opts PutOptions) (string, error) {
	if !ValidKey(key) {
		return "", ErrInvalidKey
	}
	if err := checkMeta(opts.ContentType, opts.Meta); err != nil {
		return "", err
	}
	if _, err := s.checkBucket(p, bucket, AccessWrite); err != nil {
		return "", err
	}

	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw)

	if err := os.MkdirAll(s.uploadDir(id), 0700); err != nil {
		return "", err
	}
	up := MultipartUpload{
		Id:          id,
		Bucket:      bucket,
		Key:         key,
		ContentType: opts.ContentType,
		Meta:        normalizeMeta(opts.Meta),
		Created:     time.Now().UTC(),
	}
	data, _ := json.Marshal(up)
	if err := os.WriteFile(filepath.Join(s.uploadDir(id), "upload.json"), data, 0600); err != nil {
		os.RemoveAll(s.uploadDir(id))
		return "", err
	}
	return id, nil
}

// readUpload reads the record of the upload in dir.
func readUpload(dir string) (*MultipartUpload, error) {
	data, err := os.ReadFile(filepath.Join(dir, "upload.json"))
	if err != nil {
		return nil, err
	}
	var up MultipartUpload
	if err := json.Unmarshal(data, &up); err != nil {
		return nil, err
	}
	return &up, nil
}

// loadUpload returns an upload of bucket/key that p may write to.
func (s *Store) loadUpload(p *Principal, bucket, key, id string) (*MultipartUpload, error) {
	if !validUploadId(id) {
		return nil, ErrNoSuchUpload
	}
	up, err := readUpload(s.uploadDir(id))
	if err != nil || up.Bucket != bucket || up.Key != key {
		return nil, ErrNoSuchUpload
	}
	if _, err := s.checkBucket(p, bucket, AccessWrite); err != nil {
		return nil, err
	}
	return up, nil
}

func partName(n int) string { return fmt.Sprintf("part-%05d", n) }

// UploadPart stores one part of a multipart upload, returning its ETag.
func (s *Store) UploadPart(p *Principal, bucket, key, id string, number int, r io.Reader, size int64, sha string, md5sum []byte) (string, error) {
	if number < 1 || number > 10000 {
		return "", ErrInvalidPart
	}
	if _, err := s.loadUpload(p, bucket, key, id); err != nil {
		return "", err
	}
	// Parts count against the owner's quota as they arrive, so uploads that
	// are never completed cannot fill the disk.
	owner, limit, err := s.quotaFor(p, bucket, AccessWrite)
	if err != nil {
		return "", err
	}
	pending := int64(0)
	for _, pi := range s.readParts(id) {
		if pi.Number != number {
			pending += pi.Size
		}
	}
	s.mu.RLock()
	remaining := s.remainingLocked(owner, limit, 0)
	s.mu.RUnlock()
	if remaining >= 0 {
		remaining = max(0, remaining-pending)
		if size > remaining {
			return "", ErrQuotaExceeded
		}
	}

	tw, err := s.blobs.writeTemp(r, size, remaining, sha, md5sum)
	if err != nil {
		return "", err
	}

	etag := hex.EncodeToString(tw.MD5())
	dst := filepath.Join(s.uploadDir(id), partName(number))
	if err := os.Rename(tw.Path(), dst); err != nil {
		os.Remove(tw.Path())
		return "", err
	}
	info, _ := json.Marshal(PartInfo{Number: number, ETag: etag, Size: tw.size, Modified: time.Now().UTC()})
	if err := os.WriteFile(dst+".json", info, 0600); err != nil {
		return "", err
	}
	return etag, nil
}

// UploadPartCopy fills a part of a multipart upload from bytes start to end
// (inclusive) of an existing object, end -1 meaning to its end, so a client
// can copy a large object server-side in parts. The caller needs read access
// to the source; the part counts against the destination owner's quota like
// any other. It returns the part's ETag.
func (s *Store) UploadPartCopy(ctx context.Context, p *Principal, bucket, key, id string, number int, srcBucket, srcKey string, start, end int64) (string, error) {
	if _, err := s.loadUpload(p, bucket, key, id); err != nil {
		return "", err
	}
	src, f, err := s.OpenObject(ctx, p, srcBucket, srcKey)
	if err != nil {
		return "", err
	}
	defer f.Close()

	var length int64
	switch {
	case start == 0 && end < 0: // the whole object, which may be empty
		length = src.Size
	case start >= 0 && start <= end && end < src.Size:
		length = end - start + 1
	default:
		return "", ErrInvalidRange
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	return s.UploadPart(p, bucket, key, id, number, io.LimitReader(f, length), length, "", nil)
}

// ListParts returns the parts uploaded so far.
func (s *Store) ListParts(p *Principal, bucket, key, id string) ([]PartInfo, error) {
	if _, err := s.loadUpload(p, bucket, key, id); err != nil {
		return nil, err
	}
	return s.readParts(id), nil
}

func (s *Store) readParts(id string) []PartInfo {
	matches, _ := filepath.Glob(filepath.Join(s.uploadDir(id), "part-*.json"))
	parts := make([]PartInfo, 0, len(matches))
	for _, m := range matches {
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var pi PartInfo
		if json.Unmarshal(data, &pi) == nil {
			parts = append(parts, pi)
		}
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].Number < parts[j].Number })
	return parts
}

// CompleteMultipart assembles the listed parts into the object.
func (s *Store) CompleteMultipart(p *Principal, bucket, key, id string, parts []CompletedPart, modifiedBy string) (*Object, error) {
	up, err := s.loadUpload(p, bucket, key, id)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 {
		return nil, ErrInvalidPart
	}

	have := make(map[int]PartInfo)
	for _, pi := range s.readParts(id) {
		have[pi.Number] = pi
	}

	tw, err := s.blobs.newTemp()
	if err != nil {
		return nil, err
	}
	etagHash := md5.New()
	last := 0
	for _, cp := range parts {
		pi, ok := have[cp.Number]
		if !ok || cp.Number <= last || strings.Trim(cp.ETag, `"`) != pi.ETag {
			tw.discard()
			return nil, ErrInvalidPart
		}
		last = cp.Number

		raw, _ := hex.DecodeString(pi.ETag)
		etagHash.Write(raw)

		f, err := os.Open(filepath.Join(s.uploadDir(id), partName(cp.Number)))
		if err != nil {
			tw.discard()
			return nil, ErrInvalidPart
		}
		_, err = tw.ReadFrom(f)
		f.Close()
		if err != nil {
			tw.discard()
			return nil, err
		}
	}
	if err := tw.finish(); err != nil {
		tw.discard()
		return nil, err
	}

	etag := fmt.Sprintf("%s-%d", hex.EncodeToString(etagHash.Sum(nil)), len(parts))
	_, limit, err := s.quotaFor(p, bucket, AccessWrite)
	if err != nil {
		tw.discard()
		return nil, err
	}
	o, err := s.commitContent(p, bucket, key, tw, etag, limit, PutOptions{
		ContentType: up.ContentType,
		Meta:        up.Meta,
		ModifiedBy:  modifiedBy,
	})
	if err != nil {
		return nil, err
	}
	os.RemoveAll(s.uploadDir(id))
	return o, nil
}

// AbortMultipart discards an upload and its parts.
func (s *Store) AbortMultipart(p *Principal, bucket, key, id string) error {
	if _, err := s.loadUpload(p, bucket, key, id); err != nil {
		return err
	}
	return os.RemoveAll(s.uploadDir(id))
}

// ListMultipartUploads lists the uploads in progress on this server for a bucket.
func (s *Store) ListMultipartUploads(p *Principal, bucket string) ([]*MultipartUpload, error) {
	if _, err := s.checkBucket(p, bucket, AccessWrite); err != nil {
		return nil, err
	}

	entries, _ := os.ReadDir(filepath.Join(s.dir, "multipart"))
	var out []*MultipartUpload
	for _, e := range entries {
		if up, err := readUpload(filepath.Join(s.dir, "multipart", e.Name())); err == nil && up.Bucket == bucket {
			out = append(out, up)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// cleanMultipart removes uploads that were abandoned, broken, or whose
// bucket no longer exists, returning how many it removed.
func (s *Store) cleanMultipart() int {
	removed := 0
	entries, _ := os.ReadDir(filepath.Join(s.dir, "multipart"))
	for _, e := range entries {
		dir := filepath.Join(s.dir, "multipart", e.Name())
		info, err := os.Stat(filepath.Join(dir, "upload.json"))
		if err != nil {
			// No record: a creation that never finished.
			if di, err := e.Info(); err == nil && time.Since(di.ModTime()) > time.Hour {
				if os.RemoveAll(dir) == nil {
					removed++
				}
			}
			continue
		}

		stale := time.Since(info.ModTime()) > multipartMaxAge
		if !stale {
			if up, err := readUpload(dir); err == nil {
				s.mu.RLock()
				stale = !live(s.buckets[up.Bucket])
				s.mu.RUnlock()
			}
		}
		if stale && os.RemoveAll(dir) == nil {
			removed++
		}
	}
	return removed
}
