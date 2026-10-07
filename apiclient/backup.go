package apiclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/paularlott/knot/internal/util/rest"
)

// BackupKinds lists the kinds of record in a backup, in the order a restore
// applies them. Users come last, so a server rebuilt from nothing has none
// until the rest is in place, and a restore that fails part way can be run
// again.
var BackupKinds = []string{
	"roles", "groups", "templates", "template-vars", "volumes", "scripts", "skills", "commands", "responses",
	"cfg-values", "audit-logs", "file-buckets", "file-objects", "spaces", "tokens", "users",
}

// BackupInfo describes what a server can back up.
type BackupInfo struct {
	Version string    `json:"version"`
	Time    time.Time `json:"time"`
	Kinds   []string  `json:"kinds"`
	Files   bool      `json:"files"` // the server has file storage
}

// BackupEndKey names the last line of a backup stream, which holds the number
// of records the stream carried.
const BackupEndKey = "_end"

// BackupSummary is what a backup or restore reports at its end, for the
// audit log.
type BackupSummary struct {
	Counts   map[string]int `json:"counts"`
	Warnings int            `json:"warnings"`
}

// RestoreResult is what the server did with a stream of records.
type RestoreResult struct {
	Restored int      `json:"restored"`
	Skipped  int      `json:"skipped"`
	Errors   []string `json:"errors,omitempty"`
}

// GetBackupInfo asks what the server can back up, and records the start of a
// backup in its audit log.
func (c *ApiClient) GetBackupInfo(ctx context.Context) (*BackupInfo, error) {
	info := &BackupInfo{}
	_, err := c.httpClient.Get(ctx, "/api/backup/info", info)
	return info, err
}

// BackupStream opens the records of one kind as JSON lines; the caller closes
// the stream.
func (c *ApiClient) BackupStream(ctx context.Context, kind string, params url.Values) (io.ReadCloser, error) {
	path := "/api/backup/" + url.PathEscape(kind)
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp, err := c.rawGet(ctx, path)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// BackupContent opens the content of a file by its checksum, from byte
// offset; the caller closes the body. Resumed reports whether the stream
// starts at offset, as asked, rather than at the beginning.
func (c *ApiClient) BackupContent(ctx context.Context, sha string, offset int64) (resp *http.Response, resumed bool, err error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, false, err
	}
	var headers map[string]string
	if offset > 0 {
		headers = map[string]string{"Range": fmt.Sprintf("bytes=%d-", offset)}
	}
	resp, err = hc.DoRaw(ctx, http.MethodGet, "/api/backup/content/"+url.PathEscape(sha), nil, 0, headers)
	if err != nil {
		return nil, false, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, false, rest.DecodeResponse(resp, nil)
	}
	return resp, resp.StatusCode == http.StatusPartialContent, nil
}

func (c *ApiClient) rawGet(ctx context.Context, path string) (*http.Response, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	resp, err := hc.DoRaw(ctx, http.MethodGet, path, nil, 0, nil)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		defer resp.Body.Close()
		return nil, rest.DecodeResponse(resp, nil)
	}
	return resp, nil
}

// PostBackupComplete records the end of a backup in the server's audit log.
func (c *ApiClient) PostBackupComplete(ctx context.Context, s *BackupSummary) error {
	_, err := c.httpClient.Post(ctx, "/api/backup/complete", s, nil, 200)
	return err
}

// RestoreStream sends the records of one kind, as JSON lines, to be saved.
func (c *ApiClient) RestoreStream(ctx context.Context, kind string, body io.Reader) (*RestoreResult, error) {
	hc, err := c.rawClient()
	if err != nil {
		return nil, err
	}
	resp, err := hc.DoRaw(ctx, http.MethodPost, "/api/restore/"+url.PathEscape(kind), body, -1, map[string]string{"Content-Type": "application/x-ndjson"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &RestoreResult{}
	if err := rest.DecodeResponse(resp, out); err != nil {
		return nil, err
	}
	return out, nil
}

// RestoreMissingContent returns which of the checksums the server does not
// hold the content of.
func (c *ApiClient) RestoreMissingContent(ctx context.Context, shas []string) ([]string, error) {
	req := struct {
		Shas []string `json:"shas"`
	}{shas}
	resp := struct {
		Missing []string `json:"missing"`
	}{}
	if _, err := c.httpClient.Post(ctx, "/api/restore/content/missing", &req, &resp, 200); err != nil {
		return nil, err
	}
	return resp.Missing, nil
}

// RestoreContent uploads the content with a checksum; the server refuses
// content that does not match it.
func (c *ApiClient) RestoreContent(ctx context.Context, sha string, body io.Reader, size int64) error {
	hc, err := c.rawClient()
	if err != nil {
		return err
	}
	resp, err := hc.DoRaw(ctx, http.MethodPut, "/api/restore/content/"+url.PathEscape(sha), body, size, map[string]string{"Content-Type": "application/octet-stream"})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return rest.DecodeResponse(resp, nil)
	}
	return nil
}

// PostRestoreComplete records the end of a restore in the server's audit log.
func (c *ApiClient) PostRestoreComplete(ctx context.Context, s *BackupSummary) error {
	_, err := c.httpClient.Post(ctx, "/api/restore/complete", s, nil, 200)
	return err
}
