package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

type FileGrant struct {
	Type   string `json:"type"`
	Id     string `json:"id,omitempty"`
	Name   string `json:"name,omitempty"`
	Access string `json:"access"`
}

type FileBucketInfo struct {
	Id        string      `json:"id"`           // fixed for the life of the bucket, whatever its name
	Name      string      `json:"name"`         // full name, "<owner>--<name>"
	Display   string      `json:"display_name"` // the short name for the caller's own buckets, else the full name
	OwnerId   string      `json:"owner_id"`
	OwnerName string      `json:"owner_name"`
	Grants    []FileGrant `json:"grants"`
	Size      int64       `json:"size"`
	Count     int         `json:"count"`
	Access    string      `json:"access"` // effective: owner, write or read
	// Granted is the access held in the caller's own right and Via how it is
	// held (owner, user, group, all); Via is empty when the caller reaches the
	// bucket only as a file storage manager.
	Granted   string    `json:"granted"`
	Via       string    `json:"via"`
	CreatedAt time.Time `json:"created_at"`
}

type FileBucketList struct {
	Buckets []FileBucketInfo `json:"buckets"`
}

type FileBucketCreateRequest struct {
	Name string `json:"name"`
}

// FileShareRequest shares a bucket. Type is user, group or all; Name is the
// username (or email) or group name, unused for all.
type FileShareRequest struct {
	Type   string `json:"type"`
	Name   string `json:"name"`
	Access string `json:"access"`
}

type FileUnshareRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

// FileTransferRequest moves a bucket to another user; Force allows the move
// even when it takes the new owner over their quota.
type FileTransferRequest struct {
	User  string `json:"user"`
	Force bool   `json:"force"`
}

type FileObjectInfo struct {
	Key         string    `json:"key"`
	Size        int64     `json:"size"`
	ETag        string    `json:"etag"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	ModifiedAt  time.Time `json:"modified_at"`
}

// FileChange is one entry of a bucket's change feed: a file as it is now, or
// one that was deleted.
type FileChange struct {
	FileObjectInfo
	Deleted bool `json:"deleted,omitempty"`
}

// FileChangeList is a page of a bucket's change feed. Cursor continues it;
// with More set, more changes are waiting. Reset means the cursor can no
// longer be followed (the server's index was rebuilt, the cursor came from
// another server, or deletions it had not seen were forgotten): start again
// without a cursor, which returns every file.
type FileChangeList struct {
	Changes []FileChange `json:"changes"`
	Cursor  string       `json:"cursor"`
	More    bool         `json:"more"`
	Reset   bool         `json:"reset"`
}

type FileObjectList struct {
	Objects     []FileObjectInfo `json:"objects"`
	Prefixes    []string         `json:"prefixes"`
	IsTruncated bool             `json:"is_truncated"`
	Next        string           `json:"next"`
}

// FileCopyRequest copies one file to another place, in the same bucket or
// another, on the server: the content is never transferred. The source and
// destination buckets may be named in short form, like the other endpoints.
type FileCopyRequest struct {
	SourceBucket string `json:"source_bucket"`
	SourceKey    string `json:"source_key"`
	DestBucket   string `json:"dest_bucket"`
	DestKey      string `json:"dest_key"`
	// IfMatch copies only if the destination file has this ETag;
	// IfNoneMatch only if there is no file there. Either refusal is a 412.
	IfMatch     string `json:"if_match,omitempty"`
	IfNoneMatch bool   `json:"if_none_match,omitempty"`
}

// FileMoveRequest renames a file, or a folder and everything under it, within
// a bucket, on the server. A folder is given with or without a trailing slash.
// Without Overwrite the move is refused if the destination holds a file.
type FileMoveRequest struct {
	Bucket    string `json:"bucket"`
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

// FileMoveResponse says how many files were moved.
type FileMoveResponse struct {
	Moved int `json:"moved"`
}

// FileShareTargets lists who a bucket can be shared with.
type FileShareTargets struct {
	Users  []FileShareUser  `json:"users"`
	Groups []FileShareGroup `json:"groups"`
}

type FileShareUser struct {
	Id       string `json:"user_id"`
	Username string `json:"username"`
	CanOwn   bool   `json:"can_own"` // may own buckets, so may receive a transfer
	Self     bool   `json:"self"`    // the caller
}

type FileShareGroup struct {
	Id   string `json:"group_id"`
	Name string `json:"name"`
}

type FileUsage struct {
	UsedBytes  int64 `json:"used_bytes"`
	Objects    int   `json:"objects"`
	Buckets    int   `json:"buckets"`
	QuotaBytes int64 `json:"quota_bytes"`
	MaxBuckets int   `json:"max_buckets"` // 0 for no limit
}

// FileMtimeHeader carries an object's modification time as fractional unix
// seconds, the same form rclone stores in x-amz-meta-mtime.
const FileMtimeHeader = "X-Knot-Mtime"

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
	q := url.Values{}
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
func (c *ApiClient) ReuseFileObject(ctx context.Context, bucket, key, sha string, mtime time.Time, ifMatch string, ifAbsent bool) (*FileObjectInfo, bool, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, false, err
	}
	headers := map[string]string{FileReuseHeader: sha, "Content-Type": "application/octet-stream"}
	if !mtime.IsZero() {
		headers[FileMtimeHeader] = FormatMtime(mtime)
	}
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
func (c *ApiClient) PutFileObjectIfMatch(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time, etag string) (*FileObjectInfo, error) {
	return c.putFileObjectCond(ctx, bucket, key, body, size, contentType, mtime, false, etag)
}

// PutFileObject uploads content of the given size (-1 if unknown).
func (c *ApiClient) PutFileObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time) (*FileObjectInfo, error) {
	return c.putFileObject(ctx, bucket, key, body, size, contentType, mtime, false)
}

// PutFileObjectIfAbsent is PutFileObject that leaves an existing file alone:
// it fails with an error IsPreconditionFailed recognises.
func (c *ApiClient) PutFileObjectIfAbsent(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time) (*FileObjectInfo, error) {
	return c.putFileObject(ctx, bucket, key, body, size, contentType, mtime, true)
}

// IsPreconditionFailed reports whether err is the server refusing a
// conditional write because the condition did not hold.
func IsPreconditionFailed(err error) bool {
	return rest.IsStatus(err, http.StatusPreconditionFailed)
}

func (c *ApiClient) putFileObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time, ifAbsent bool) (*FileObjectInfo, error) {
	return c.putFileObjectCond(ctx, bucket, key, body, size, contentType, mtime, ifAbsent, "")
}

func (c *ApiClient) putFileObjectCond(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time, ifAbsent bool, ifMatch string) (*FileObjectInfo, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}

	headers := map[string]string{"Content-Type": contentType}
	if contentType == "" {
		headers["Content-Type"] = "application/octet-stream"
	}
	if !mtime.IsZero() {
		headers[FileMtimeHeader] = FormatMtime(mtime)
	}
	if ifAbsent {
		headers["If-None-Match"] = "*"
	}
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}

	resp, err := hc.DoRaw(ctx, http.MethodPut, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), body, size, headers)
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

// GetFileObject downloads an object; the caller closes the body.
func (c *ApiClient) GetFileObject(ctx context.Context, bucket, key string) (*http.Response, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	resp, err := hc.DoRaw(ctx, http.MethodGet, "/api/files/objects/"+url.PathEscape(bucket)+"/"+escapeKey(key), nil, 0, nil)
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
