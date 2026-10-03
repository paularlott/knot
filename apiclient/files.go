package apiclient

import (
	"context"
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

type FileObjectList struct {
	Objects     []FileObjectInfo `json:"objects"`
	Prefixes    []string         `json:"prefixes"`
	IsTruncated bool             `json:"is_truncated"`
	Next        string           `json:"next"`
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

func (c *ApiClient) rawClient() (*rest.HTTPClient, error) {
	hc, ok := c.httpClient.(*rest.HTTPClient)
	if !ok {
		return nil, fmt.Errorf("file transfer requires an HTTP client")
	}
	return hc, nil
}

// PutFileObject uploads content of the given size (-1 if unknown).
func (c *ApiClient) PutFileObject(ctx context.Context, bucket, key string, body io.Reader, size int64, contentType string, mtime time.Time) (*FileObjectInfo, error) {
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
