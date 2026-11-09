package apiclient

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

type FileGrant struct {
	Type   string `json:"type" msgpack:"type"`
	Id     string `json:"id,omitempty" msgpack:"id,omitempty"`
	Name   string `json:"name,omitempty" msgpack:"name,omitempty"`
	Access string `json:"access" msgpack:"access"`
}

type FileBucketInfo struct {
	Id        string      `json:"id" msgpack:"id"`                     // fixed for the life of the bucket, whatever its name
	Name      string      `json:"name" msgpack:"name"`                 // full name, "<owner>--<name>"
	Display   string      `json:"display_name" msgpack:"display_name"` // the short name for the caller's own buckets, else the full name
	OwnerId   string      `json:"owner_id" msgpack:"owner_id"`
	OwnerName string      `json:"owner_name" msgpack:"owner_name"`
	Grants    []FileGrant `json:"grants" msgpack:"grants"`
	Size      int64       `json:"size" msgpack:"size"`
	Count     int         `json:"count" msgpack:"count"`
	Access    string      `json:"access" msgpack:"access"` // effective: owner, write or read
	// Granted is the access held in the caller's own right and Via how it is
	// held (owner, user, group, all); Via is empty when the caller reaches the
	// bucket only as a file storage manager.
	Granted   string    `json:"granted" msgpack:"granted"`
	Via       string    `json:"via" msgpack:"via"`
	CreatedAt time.Time `json:"created_at" msgpack:"created_at"`
}

type FileBucketList struct {
	Buckets []FileBucketInfo `json:"buckets" msgpack:"buckets"`
}

type FileBucketCreateRequest struct {
	Name string `json:"name" msgpack:"name"`
}

// FileShareRequest shares a bucket. Type is user, group or all; Name is the
// username (or email) or group name, unused for all.
type FileShareRequest struct {
	Type   string `json:"type" msgpack:"type"`
	Name   string `json:"name" msgpack:"name"`
	Access string `json:"access" msgpack:"access"`
}

type FileUnshareRequest struct {
	Type string `json:"type" msgpack:"type"`
	Name string `json:"name" msgpack:"name"`
}

// FileTransferRequest moves a bucket to another user; Force allows the move
// even when it takes the new owner over their quota.
type FileTransferRequest struct {
	User  string `json:"user" msgpack:"user"`
	Force bool   `json:"force" msgpack:"force"`
}

type FileObjectInfo struct {
	Key         string    `json:"key" msgpack:"key"`
	Size        int64     `json:"size" msgpack:"size"`
	ETag        string    `json:"etag" msgpack:"etag"`
	SHA256      string    `json:"sha256" msgpack:"sha256"`
	ContentType string    `json:"content_type" msgpack:"content_type"`
	ModifiedAt  time.Time `json:"modified_at" msgpack:"modified_at"`
	// ChangedAt is when this version of the file was saved, its content
	// or metadata changed: unlike ModifiedAt, not the time a client gave
	// it. Zero from servers that do not say.
	ChangedAt time.Time `json:"changed_at,omitzero" msgpack:"changed_at,omitempty"`
	// Meta is the file's user metadata, as S3 x-amz-meta-*: its mtime and,
	// for a file written over NFS, its mode and inode number.
	Meta map[string]string `json:"meta,omitempty" msgpack:"meta,omitempty"`
}

// FileChange is one entry of a bucket's change feed: a file as it is now, or
// one that was deleted.
type FileChange struct {
	FileObjectInfo
	Deleted bool `json:"deleted,omitempty" msgpack:"deleted,omitempty"`
}

// FileChangeList is a page of a bucket's change feed. Cursor continues it;
// with More set, more changes are waiting. Reset means the cursor can no
// longer be followed (the server's index was rebuilt, the cursor came from
// another server, or deletions it had not seen were forgotten): start again
// without a cursor, which returns every file.
type FileChangeList struct {
	Changes []FileChange `json:"changes" msgpack:"changes"`
	Cursor  string       `json:"cursor" msgpack:"cursor"`
	More    bool         `json:"more" msgpack:"more"`
	Reset   bool         `json:"reset" msgpack:"reset"`
}

// FileStat says what a path in a bucket is: a file, or a folder, with its
// marker (the object that keeps an empty folder) when it has one.
type FileStat struct {
	File   *FileObjectInfo `json:"file,omitempty" msgpack:"file,omitempty"`
	Folder bool            `json:"folder,omitempty" msgpack:"folder,omitempty"`
	Marker *FileObjectInfo `json:"marker,omitempty" msgpack:"marker,omitempty"`
}

// FileRange is part of a file to write, for PatchFileObject.
type FileRange struct {
	Offset int64
	Length int64
	Data   io.Reader // Length bytes
}

type FileObjectList struct {
	Objects     []FileObjectInfo `json:"objects" msgpack:"objects"`
	Prefixes    []string         `json:"prefixes" msgpack:"prefixes"`
	IsTruncated bool             `json:"is_truncated" msgpack:"is_truncated"`
	Next        string           `json:"next" msgpack:"next"`
	// Markers, when asked for, are the markers of the folders in Prefixes
	// that have one, by prefix.
	Markers map[string]FileObjectInfo `json:"markers,omitempty" msgpack:"markers,omitempty"`
}

// FileCopyRequest copies one file to another place, in the same bucket or
// another, on the server: the content is never transferred. The source and
// destination buckets may be named in short form, like the other endpoints.
type FileCopyRequest struct {
	SourceBucket string `json:"source_bucket" msgpack:"source_bucket"`
	SourceKey    string `json:"source_key" msgpack:"source_key"`
	DestBucket   string `json:"dest_bucket" msgpack:"dest_bucket"`
	DestKey      string `json:"dest_key" msgpack:"dest_key"`
	// IfMatch copies only if the destination file has this ETag;
	// IfNoneMatch only if there is no file there. Either refusal is a 412.
	IfMatch     string `json:"if_match,omitempty" msgpack:"if_match,omitempty"`
	IfNoneMatch bool   `json:"if_none_match,omitempty" msgpack:"if_none_match,omitempty"`
	// Meta, when set, replaces the copy's metadata. With it, a file may be
	// copied onto itself to change only its metadata.
	Meta map[string]string `json:"meta,omitempty" msgpack:"meta,omitempty"`
}

// FileMoveRequest renames a file, or a folder and everything under it, within
// a bucket, on the server. A folder is given with or without a trailing slash.
// Without Overwrite the move is refused if the destination holds a file.
type FileMoveRequest struct {
	Bucket    string `json:"bucket" msgpack:"bucket"`
	From      string `json:"from" msgpack:"from"`
	To        string `json:"to" msgpack:"to"`
	Overwrite bool   `json:"overwrite,omitempty" msgpack:"overwrite,omitempty"`
}

// FileMoveResponse says how many files were moved.
type FileMoveResponse struct {
	Moved int `json:"moved" msgpack:"moved"`
}

// FileSnapshotRequest copies a folder and everything below it, as it was at
// one moment, to a new folder, on the server: no content is transferred.
// Source "" is the whole bucket. The destination must not exist.
type FileSnapshotRequest struct {
	SourceBucket string `json:"source_bucket" msgpack:"source_bucket"`
	Source       string `json:"source" msgpack:"source"`
	DestBucket   string `json:"dest_bucket" msgpack:"dest_bucket"`
	Dest         string `json:"dest" msgpack:"dest"`
}

// FileSnapshot is a snapshot made, as its manifest records it.
type FileSnapshot struct {
	Files        int       `json:"files" msgpack:"files"`
	Folders      int       `json:"folders" msgpack:"folders"`
	Bytes        int64     `json:"bytes" msgpack:"bytes"`
	TakenAt      time.Time `json:"taken_at" msgpack:"taken_at"`
	SourceBucket string    `json:"source_bucket" msgpack:"source_bucket"`
	SourcePath   string    `json:"source_path" msgpack:"source_path"`
	// Manifest is the key of the manifest written last into the snapshot.
	Manifest string `json:"manifest" msgpack:"manifest"`
}

// FileShareTargets lists who a bucket can be shared with.
type FileShareTargets struct {
	Users  []FileShareUser  `json:"users" msgpack:"users"`
	Groups []FileShareGroup `json:"groups" msgpack:"groups"`
}

type FileShareUser struct {
	Id       string `json:"user_id" msgpack:"user_id"`
	Username string `json:"username" msgpack:"username"`
	CanOwn   bool   `json:"can_own" msgpack:"can_own"` // may own buckets, so may receive a transfer
	Self     bool   `json:"self" msgpack:"self"`       // the caller
}

type FileShareGroup struct {
	Id   string `json:"group_id" msgpack:"group_id"`
	Name string `json:"name" msgpack:"name"`
}

type FileUsage struct {
	UsedBytes  int64 `json:"used_bytes" msgpack:"used_bytes"`
	Objects    int   `json:"objects" msgpack:"objects"`
	Buckets    int   `json:"buckets" msgpack:"buckets"`
	QuotaBytes int64 `json:"quota_bytes" msgpack:"quota_bytes"`
	MaxBuckets int   `json:"max_buckets" msgpack:"max_buckets"` // 0 for no limit
}

// FileMtimeHeader carries an object's modification time as fractional unix
// seconds, the same form rclone stores in x-amz-meta-mtime.
const FileMtimeHeader = "X-Knot-Mtime"

// FileChangedHeader carries when an object's version was saved (see
// FileObjectInfo.ChangedAt), as FileMtimeHeader does its modification time.
const FileChangedHeader = "X-Knot-Changed"

// FileMetaHeaderPrefix starts the headers carrying an object's user metadata
// on GET, HEAD and PUT, one header per key (cf. S3's x-amz-meta-).
const FileMetaHeaderPrefix = "X-Knot-Meta-"

// FileModeMeta is the metadata key holding a file's mode: the POSIX st_mode,
// file type and permissions, in octal as rclone --metadata writes it
// ("100755"), so modes round-trip through rclone. Knot before 0.42 wrote the
// decimal form s3fs uses; knot admin file fsck --repair rewrites those.
const FileModeMeta = "mode"

// POSIX st_mode file types, the same on every platform knot runs on.
const (
	stIFMT  = 0o170000
	stIFREG = 0o100000
	stIFDIR = 0o040000
	stIFLNK = 0o120000
)

// FormatFileMode renders a mode as FileModeMeta holds it.
func FormatFileMode(mode os.FileMode) string {
	typ := uint64(stIFREG)
	switch {
	case mode&os.ModeSymlink != 0:
		typ = stIFLNK
	case mode&os.ModeDir != 0:
		typ = stIFDIR
	}
	return strconv.FormatUint(typ|uint64(mode&os.ModePerm), 8)
}

// ParseFileMode reads the mode kept in meta: its permission bits, with
// os.ModeSymlink for a link. ok is false when meta holds none, or holds
// something that is not a file's, folder's or link's mode.
func ParseFileMode(meta map[string]string) (mode os.FileMode, ok bool) {
	v, found := meta[FileModeMeta]
	if !found {
		return 0, false
	}
	m, err := strconv.ParseUint(strings.TrimSpace(v), 8, 32)
	if err != nil || m > 0o177777 {
		return 0, false
	}
	switch m & stIFMT {
	case 0, stIFREG, stIFDIR:
	case stIFLNK:
		mode = os.ModeSymlink
	default:
		return 0, false
	}
	return mode | os.FileMode(m)&os.ModePerm, true
}

// FileAttrs are the attributes an upload records with a file's content. A
// zero field is not recorded: an upload without a mode keeps the mode of the
// file it replaces.
type FileAttrs struct {
	Mtime time.Time
	Mode  os.FileMode // permission bits, with os.ModeSymlink for a link
}

// setHeaders adds the attributes to an upload's headers.
func (a FileAttrs) setHeaders(headers map[string]string) {
	if !a.Mtime.IsZero() {
		headers[FileMtimeHeader] = FormatMtime(a.Mtime)
	}
	if a.Mode != 0 {
		headers[FileMetaHeaderPrefix+FileModeMeta] = FormatFileMode(a.Mode)
	}
}

// LocalFileAttrs are the attributes of a local file an upload records: its
// modification time and, where the platform has them, its permission bits.
func LocalFileAttrs(info os.FileInfo) FileAttrs {
	attrs := FileAttrs{Mtime: info.ModTime()}
	if runtime.GOOS != "windows" {
		attrs.Mode = info.Mode().Perm()
	}
	return attrs
}

// FileWriterHeader names the client making a change, so change events can
// say who made each (see SetWriter).
const FileWriterHeader = "X-Knot-Writer"

// SetWriter names this client in the changes it makes, so in the change
// events (FilesChangedPayload) it can tell its own changes from others'.
func (c *ApiClient) SetWriter(id string) {
	if hc, ok := c.httpClient.(*rest.HTTPClient); ok {
		hc.SetHeader(FileWriterHeader, id)
	}
}

// FileSizeHeader gives a patched file's new size.
const FileSizeHeader = "X-Knot-Size"

// FilePatchContentType is the body of a patch (see PatchFileObject).
const FilePatchContentType = "application/x-knot-patch"

// FileReuseHeader on an upload with no body names the SHA-256 of content the
// bucket held until recently (a file deleted or replaced within the hour):
// the file is written with that content without it being sent. The server
// answers 409 when it does not hold the content, and the client sends it.
const FileReuseHeader = "X-Knot-Reuse-Sha256"

func escapeKey(key string) string {
	parts := strings.Split(key, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

func (c *ApiClient) GetFileBuckets(ctx context.Context, all bool) (*FileBucketList, error) {
	path := "/api/files/buckets"
	if all {
		path += "?all=true"
	}
	response := &FileBucketList{}
	_, err := c.httpClient.Get(ctx, path, response)
	return response, err
}

func (c *ApiClient) GetFileBucket(ctx context.Context, bucket string) (*FileBucketInfo, error) {
	response := &FileBucketInfo{}
	_, err := c.httpClient.Get(ctx, "/api/files/buckets/"+url.PathEscape(bucket), response)
	return response, err
}

func (c *ApiClient) CreateFileBucket(ctx context.Context, name string) (*FileBucketInfo, error) {
	response := &FileBucketInfo{}
	_, err := c.httpClient.Post(ctx, "/api/files/buckets", &FileBucketCreateRequest{Name: name}, response, 201)
	return response, err
}

func (c *ApiClient) DeleteFileBucket(ctx context.Context, bucket string, force bool) error {
	path := "/api/files/buckets/" + url.PathEscape(bucket)
	if force {
		path += "?force=true"
	}
	_, err := c.httpClient.Delete(ctx, path, nil, nil, 200)
	return err
}

func (c *ApiClient) ShareFileBucket(ctx context.Context, bucket string, req FileShareRequest) (*FileBucketInfo, error) {
	response := &FileBucketInfo{}
	_, err := c.httpClient.Post(ctx, "/api/files/buckets/"+url.PathEscape(bucket)+"/share", &req, response, 200)
	return response, err
}

func (c *ApiClient) UnshareFileBucket(ctx context.Context, bucket string, req FileUnshareRequest) (*FileBucketInfo, error) {
	response := &FileBucketInfo{}
	_, err := c.httpClient.Post(ctx, "/api/files/buckets/"+url.PathEscape(bucket)+"/unshare", &req, response, 200)
	return response, err
}

func (c *ApiClient) TransferFileBucket(ctx context.Context, bucket string, user string, force bool) (*FileBucketInfo, error) {
	response := &FileBucketInfo{}
	_, err := c.httpClient.Post(ctx, "/api/files/buckets/"+url.PathEscape(bucket)+"/transfer", &FileTransferRequest{User: user, Force: force}, response, 200)
	return response, err
}

func (c *ApiClient) GetFileUsage(ctx context.Context) (*FileUsage, error) {
	response := &FileUsage{}
	_, err := c.httpClient.Get(ctx, "/api/files/usage", response)
	return response, err
}

// ListFileObjects lists one page of a bucket. With delimiter "/" keys below
// the next "/" are rolled up into prefixes.
func (c *ApiClient) ListFileObjects(ctx context.Context, bucket, prefix, delimiter, after string, limit int) (*FileObjectList, error) {
	return c.listFileObjects(ctx, bucket, prefix, delimiter, after, limit, false)
}

func (c *ApiClient) listFileObjects(ctx context.Context, bucket, prefix, delimiter, after string, limit int, markers bool) (*FileObjectList, error) {
	q := url.Values{}
	if markers {
		q.Set("markers", "1")
	}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if delimiter != "" {
		q.Set("delimiter", delimiter)
	}
	if after != "" {
		q.Set("after", after)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/files/list/" + url.PathEscape(bucket)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	response := &FileObjectList{}
	_, err := c.httpClient.Get(ctx, path, response)
	return response, err
}

func (c *ApiClient) DeleteFileObject(ctx context.Context, bucket, key string) error {
	_, err := c.httpClient.Delete(ctx, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, nil, 200)
	return err
}

// DeleteFileObjectIfMatch deletes a file only if it still has the ETag etag:
// otherwise it fails with an error IsPreconditionFailed recognises.
func (c *ApiClient) DeleteFileObjectIfMatch(ctx context.Context, bucket, key, etag string) error {
	hc, err := c.rawClient()
	if err != nil {
		return err
	}
	resp, err := hc.DoRaw(ctx, http.MethodDelete, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, map[string]string{"If-Match": `"` + etag + `"`})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return rest.DecodeResponse(resp, nil)
}

// CopyFileObject copies a file on the server, keeping its content type,
// metadata and modification time, and replacing any file at the destination.
func (c *ApiClient) CopyFileObject(ctx context.Context, req FileCopyRequest) (*FileObjectInfo, error) {
	response := &FileObjectInfo{}
	_, err := c.httpClient.Post(ctx, "/api/files/copy", &req, response, 200)
	return response, err
}

// MoveFileObjects renames a file, or a folder and its contents, within a
// bucket, on the server and without transferring any content. It returns how
// many files moved.
func (c *ApiClient) MoveFileObjects(ctx context.Context, req FileMoveRequest) (int, error) {
	response := &FileMoveResponse{}
	_, err := c.httpClient.Post(ctx, "/api/files/move", &req, response, 200)
	return response.Moved, err
}

// SnapshotFileFolder copies a folder, as it was at one moment, to a new
// folder on the server, without transferring any content.
func (c *ApiClient) SnapshotFileFolder(ctx context.Context, req FileSnapshotRequest) (*FileSnapshot, error) {
	response := &FileSnapshot{}
	_, err := c.httpClient.Post(ctx, "/api/files/snapshot", &req, response, 200)
	return response, err
}

// FilesFsck asks the server to check file storage, with no time limit as a
// deep check of a large store takes a while. req and out are a
// filestore.FsckOptions and filestore.FsckReport; it needs the permission to
// manage file storage.
func (c *ApiClient) FilesFsck(ctx context.Context, req, out any) error {
	hc, err := c.rawClient()
	if err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := hc.DoRaw(ctx, http.MethodPost, "/api/files/fsck", bytes.NewReader(body), int64(len(body)), map[string]string{"Content-Type": "application/json"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return rest.DecodeResponse(resp, out)
}

func (c *ApiClient) rawClient() (*rest.HTTPClient, error) {
	hc, ok := c.httpClient.(*rest.HTTPClient)
	if !ok {
		return nil, fmt.Errorf("file transfer requires an HTTP client")
	}
	return hc, nil
}

// FileCursorNow, as the cursor of ListFileChanges, returns no changes, only
// a cursor to follow the bucket from now on.
const FileCursorNow = "now"

// ListFileChanges returns a page of what changed in a bucket under prefix
// since cursor; with no cursor, every file and a cursor to follow on from.
func (c *ApiClient) ListFileChanges(ctx context.Context, bucket, prefix, cursor string, limit int) (*FileChangeList, error) {
	q := url.Values{}
	if prefix != "" {
		q.Set("prefix", prefix)
	}
	if cursor != "" {
		q.Set("cursor", cursor)
	}
	if limit > 0 {
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/api/files/changes/" + url.PathEscape(bucket)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	out := &FileChangeList{}
	if _, err := c.httpClient.Get(ctx, path, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ReuseFileObject writes a file with content the bucket held until recently,
// without sending it. ok is false when the server does not hold the content:
// the caller uploads it instead. ifMatch, when set, is the ETag the file
// must still have; ifAbsent writes only if there is no file.
func (c *ApiClient) ReuseFileObject(ctx context.Context, bucket, key, sha string, attrs FileAttrs, ifMatch string, ifAbsent bool) (*FileObjectInfo, bool, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, false, err
	}
	headers := map[string]string{FileReuseHeader: sha, "Content-Type": "application/octet-stream"}
	attrs.setHeaders(headers)
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}
	if ifAbsent {
		headers["If-None-Match"] = "*"
	}
	resp, err := hc.DoRaw(ctx, http.MethodPut, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, headers)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict {
		io.Copy(io.Discard, resp.Body)
		return nil, false, nil
	}
	info := &FileObjectInfo{}
	if err := rest.DecodeResponse(resp, info); err != nil {
		return nil, false, err
	}
	return info, true, nil
}

// PutFileObjectIfMatch uploads content only if the file still has the ETag
// etag: otherwise it fails with an error IsPreconditionFailed recognises.
func (c *ApiClient) PutFileObjectIfMatch(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, attrs FileAttrs, etag string) (*FileObjectInfo, error) {
	return c.putFileObjectCond(ctx, bucket, key, body, size, contentType, attrs, false, etag, nil)
}

// PutFileObject uploads content of the given size (-1 if unknown).
func (c *ApiClient) PutFileObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, attrs FileAttrs) (*FileObjectInfo, error) {
	return c.putFileObject(ctx, bucket, key, body, size, contentType, attrs, false)
}

// PutFileObjectIfAbsent is PutFileObject that leaves an existing file alone:
// it fails with an error IsPreconditionFailed recognises.
func (c *ApiClient) PutFileObjectIfAbsent(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, attrs FileAttrs) (*FileObjectInfo, error) {
	return c.putFileObject(ctx, bucket, key, body, size, contentType, attrs, true)
}

// IsPreconditionFailed reports whether err is the server refusing a
// conditional write because the condition did not hold.
func IsPreconditionFailed(err error) bool {
	return rest.IsStatus(err, http.StatusPreconditionFailed)
}

func (c *ApiClient) putFileObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, attrs FileAttrs, ifAbsent bool) (*FileObjectInfo, error) {
	return c.putFileObjectCond(ctx, bucket, key, body, size, contentType, attrs, ifAbsent, "", nil)
}

// PutFileObjectMeta uploads an object with user metadata (FileObjectInfo.Meta).
func (c *ApiClient) PutFileObjectMeta(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, meta map[string]string) (*FileObjectInfo, error) {
	return c.putFileObjectCond(ctx, bucket, key, body, size, contentType, FileAttrs{}, false, "", meta)
}

func (c *ApiClient) putFileObjectCond(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, attrs FileAttrs, ifAbsent bool, ifMatch string, meta map[string]string) (*FileObjectInfo, error) {

	headers := map[string]string{"Content-Type": contentType}
	for k, v := range meta {
		headers[FileMetaHeaderPrefix+k] = v
	}
	if contentType == "" {
		headers["Content-Type"] = "application/octet-stream"
	}
	attrs.setHeaders(headers)
	if ifAbsent {
		headers["If-None-Match"] = "*"
	}
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}

	resp, err := c.doTransfer(ctx, http.MethodPut, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), body, size, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	info := &FileObjectInfo{}
	if err := rest.DecodeResponse(resp, info); err != nil {
		return nil, err
	}
	return info, nil
}

// StatFileObject says what a path in a bucket is, in one request; a path
// that is neither a file nor a folder is an error, as a missing object is.
func (c *ApiClient) StatFileObject(ctx context.Context, bucket, key string) (*FileStat, error) {
	out := &FileStat{}
	_, err := c.httpClient.Get(ctx, "/api/files/stat/"+url.PathEscape(bucket)+"/"+escapeKey(key), out)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListFileObjectsMarkers lists as ListFileObjects does, with the marker of
// each folder rolled up into a prefix (FileObjectList.Markers).
func (c *ApiClient) ListFileObjectsMarkers(ctx context.Context, bucket, prefix, delimiter, after string, limit int) (*FileObjectList, error) {
	return c.listFileObjects(ctx, bucket, prefix, delimiter, after, limit, true)
}

// PatchFileObject changes part of an object: the ranges, in order and not
// overlapping, are written over it and it is cut or zero-filled to size, on
// the server, so only what changed is sent. With ifMatch, only if the object
// still has that ETag.
func (c *ApiClient) PatchFileObject(ctx context.Context, bucket, key string, ranges []FileRange, size int64, ifMatch, contentType string, meta map[string]string) (*FileObjectInfo, error) {
	var parts []io.Reader
	var length int64
	for _, rg := range ranges {
		var hdr [16]byte
		binary.BigEndian.PutUint64(hdr[:8], uint64(rg.Offset))
		binary.BigEndian.PutUint64(hdr[8:], uint64(rg.Length))
		parts = append(parts, bytes.NewReader(hdr[:]), io.LimitReader(rg.Data, rg.Length))
		length += 16 + rg.Length
	}
	headers := map[string]string{
		"Content-Type": FilePatchContentType,
		FileSizeHeader: strconv.FormatInt(size, 10),
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}
	for k, v := range meta {
		headers[FileMetaHeaderPrefix+k] = v
	}
	resp, err := c.doTransfer(ctx, http.MethodPatch, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), io.MultiReader(parts...), length, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	info := &FileObjectInfo{}
	if err := rest.DecodeResponse(resp, info); err != nil {
		return nil, err
	}
	return info, nil
}

// HeadFileObject describes an object without downloading it.
func (c *ApiClient) HeadFileObject(ctx context.Context, bucket, key string) (*FileObjectInfo, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	resp, err := hc.DoRaw(ctx, http.MethodHead, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, rest.DecodeResponse(resp, nil)
	}
	return ObjectInfoFromHeaders(key, resp.Header, resp.ContentLength), nil
}

// ObjectInfoFromHeaders reads the description of an object from the headers
// of a GET or HEAD of it.
func ObjectInfoFromHeaders(key string, h http.Header, size int64) *FileObjectInfo {
	info := &FileObjectInfo{
		Key:         key,
		Size:        size,
		ETag:        strings.Trim(h.Get("ETag"), `"`),
		SHA256:      h.Get("X-Knot-Sha256"),
		ContentType: h.Get("Content-Type"),
	}
	lastModified, lmErr := http.ParseTime(h.Get("Last-Modified"))
	if t, ok := ParseMtime(h.Get(FileMtimeHeader)); ok {
		info.ModifiedAt = t
	} else if lmErr == nil {
		info.ModifiedAt = lastModified
	}
	if t, ok := ParseMtime(h.Get(FileChangedHeader)); ok {
		info.ChangedAt = t
	} else if lmErr == nil {
		info.ChangedAt = lastModified
	}
	for k, v := range h {
		if len(v) > 0 && len(k) > len(FileMetaHeaderPrefix) && strings.EqualFold(k[:len(FileMetaHeaderPrefix)], FileMetaHeaderPrefix) {
			if info.Meta == nil {
				info.Meta = map[string]string{}
			}
			info.Meta[strings.ToLower(k[len(FileMetaHeaderPrefix):])] = v[0]
		}
	}
	return info
}

// GetFileObjectRange downloads length bytes of an object from offset; the
// caller closes the body.
//
// With ifMatch, an ETag, it is read only if the object still has it, a
// 412 error otherwise.
func (c *ApiClient) GetFileObjectRange(ctx context.Context, bucket, key string, offset, length int64, ifMatch string) (*http.Response, error) {
	headers := map[string]string{"Range": fmt.Sprintf("bytes=%d-%d", offset, offset+length-1)}
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}
	resp, err := c.doTransfer(ctx, http.MethodGet, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, headers)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		resp.Body.Close()
		return nil, io.EOF
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, rest.DecodeResponse(resp, nil)
	}
	return resp, nil
}

// GetFileObject downloads an object; the caller closes the body.
func (c *ApiClient) GetFileObject(ctx context.Context, bucket, key string) (*http.Response, error) {
	return c.GetFileObjectIf(ctx, bucket, key, "")
}

// GetFileObjectIf downloads an object if it still has the ETag ifMatch (any
// version when empty), a 412 error otherwise; the caller closes the body.
func (c *ApiClient) GetFileObjectIf(ctx context.Context, bucket, key, ifMatch string) (*http.Response, error) {
	var headers map[string]string
	if ifMatch != "" {
		headers = map[string]string{"If-Match": `"` + ifMatch + `"`}
	}
	resp, err := c.doTransfer(ctx, http.MethodGet, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, headers)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, rest.DecodeResponse(resp, nil)
	}
	return resp, nil
}

// FormatMtime renders a time as fractional unix seconds.
func FormatMtime(t time.Time) string {
	return strconv.FormatFloat(float64(t.UnixNano())/1e9, 'f', 9, 64)
}

// ParseMtime parses fractional unix seconds or RFC 3339.
func ParseMtime(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		sec := int64(f)
		return time.Unix(sec, int64((f-float64(sec))*1e9)), true
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}
