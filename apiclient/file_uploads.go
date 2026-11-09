package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"

	"github.com/paularlott/knot/internal/util/rest"
)

// A file can be uploaded in parts: the upload is started, its parts sent
// in any order and again if need be, and completed, when the parts become
// the file at once. A part sent is kept on the server until the upload
// completes or is abandoned (after a week), so an upload stopped part way,
// by a lost connection or the knot command being interrupted, can carry on
// from the parts already there rather than start again. The parts are held
// by the server that received them: an upload is carried on through the
// same server.

// FileUploadPartHeader carries a part's SHA-256 checksum, so the server
// refuses a part that arrived damaged.
const FileUploadPartHeader = "X-Knot-Part-Sha256"

// FileUpload is an upload in parts, as started.
type FileUpload struct {
	UploadId string `json:"upload_id" msgpack:"upload_id"`
}

// FileUploadPart is a part of an upload the server holds.
type FileUploadPart struct {
	Number int    `json:"number" msgpack:"number"`
	ETag   string `json:"etag" msgpack:"etag"`
	Size   int64  `json:"size" msgpack:"size"`
}

// FileUploadParts lists the parts of an upload the server holds.
type FileUploadParts struct {
	Parts []FileUploadPart `json:"parts" msgpack:"parts"`
}

// FileUploadComplete names the parts that make the file, in order.
type FileUploadComplete struct {
	Parts []FileUploadPart `json:"parts" msgpack:"parts"`
}

func uploadPath(bucket, key string) string {
	return "/api/files/uploads/" + url.PathEscape(bucket) + "/" + escapeKey(key)
}

// StartFileUpload starts an upload in parts of a file of the bucket, with
// the attributes and metadata it is to have, as PutFileObject takes them.
func (c *ApiClient) StartFileUpload(ctx context.Context, bucket, key, contentType string, attrs FileAttrs, meta map[string]string) (string, error) {
	hc, err := c.rawClient()
	if err != nil {
		return "", err
	}
	headers := map[string]string{}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	for k, v := range meta {
		headers[FileMetaHeaderPrefix+k] = v
	}
	attrs.setHeaders(headers)
	resp, err := hc.DoRaw(ctx, http.MethodPost, uploadPath(bucket, key), nil, 0, headers)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	out := &FileUpload{}
	if err := rest.DecodeResponse(resp, out); err != nil {
		return "", err
	}
	return out.UploadId, nil
}

// UploadFilePart sends part number of an upload, numbered from 1; with
// sha, its hex SHA-256, the server refuses it if it does not match. A part
// sent again replaces the one held.
func (c *ApiClient) UploadFilePart(ctx context.Context, bucket, key, uploadId string, number int, body io.Reader, size int64, sha string) (*FileUploadPart, error) {
	q := url.Values{"upload_id": {uploadId}, "part": {strconv.Itoa(number)}}
	headers := map[string]string{"Content-Type": "application/octet-stream"}
	if sha != "" {
		headers[FileUploadPartHeader] = sha
	}
	resp, err := c.doTransfer(ctx, http.MethodPut, uploadPath(bucket, key)+"?"+q.Encode(), body, size, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &FileUploadPart{}
	if err := rest.DecodeResponse(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListFileUploadParts returns the parts of an upload the server holds; an
// upload it does not know is an error IsNotFound recognises.
func (c *ApiClient) ListFileUploadParts(ctx context.Context, bucket, key, uploadId string) ([]FileUploadPart, error) {
	out := &FileUploadParts{}
	if _, err := c.httpClient.Get(ctx, uploadPath(bucket, key)+"?"+url.Values{"upload_id": {uploadId}}.Encode(), out); err != nil {
		return nil, err
	}
	return out.Parts, nil
}

// CompleteFileUpload makes the parts, in order, the file. With ifMatch,
// only if the file there still has that ETag; with ifAbsent, only if there
// is none: either way failing with an error IsPreconditionFailed
// recognises, as PutFileObjectIfMatch does.
func (c *ApiClient) CompleteFileUpload(ctx context.Context, bucket, key, uploadId string, parts []FileUploadPart, ifMatch string, ifAbsent bool) (*FileObjectInfo, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(&FileUploadComplete{Parts: parts})
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Content-Type": "application/json"}
	if ifMatch != "" {
		headers["If-Match"] = `"` + ifMatch + `"`
	}
	if ifAbsent {
		headers["If-None-Match"] = "*"
	}
	q := url.Values{"upload_id": {uploadId}, "complete": {"1"}}
	// Assembling a large file takes the server a while: no timeout but
	// ctx's.
	resp, err := hc.DoRaw(rest.WithoutTimeout(ctx), http.MethodPost, uploadPath(bucket, key)+"?"+q.Encode(), bytes.NewReader(body), int64(len(body)), headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &FileObjectInfo{}
	if err := rest.DecodeResponse(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

// AbortFileUpload discards an upload and the parts held.
func (c *ApiClient) AbortFileUpload(ctx context.Context, bucket, key, uploadId string) error {
	_, err := c.httpClient.Delete(ctx, uploadPath(bucket, key)+"?"+url.Values{"upload_id": {uploadId}}.Encode(), nil, nil, 200)
	return err
}
