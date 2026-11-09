package api

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/util/rest"
)

// Uploads in parts (apiclient's StartFileUpload and those after it): a large
// file is sent a part at a time, each kept on this server until the upload
// completes, so an upload stopped part way carries on from the parts already
// here. Completing it is a write of the whole file, with the same conditions
// and attributes a PUT has.

// HandleStartFileUpload starts an upload in parts (POST without upload_id)
// or completes one (POST with upload_id and complete).
func HandleStartFileUpload(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("upload_id") != "" {
		handleCompleteFileUpload(w, r)
		return
	}
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	id, err := store.CreateMultipart(p, bucket, key, filestore.PutOptions{
		ContentType: contentType,
		Meta:        metaFromRequest(r),
	})
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, apiclient.FileUpload{UploadId: id})
}

// HandleUploadFilePart takes one part of an upload.
func HandleUploadFilePart(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	rest.NoDeadlines(w)
	q := r.URL.Query()
	number, err := strconv.Atoi(q.Get("part"))
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "part must be a number"})
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	sha := strings.ToLower(r.Header.Get(apiclient.FileUploadPartHeader))
	etag, err := store.UploadPart(p, bucket, key, q.Get("upload_id"), number, r.Body, r.ContentLength, sha, nil)
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, apiclient.FileUploadPart{Number: number, ETag: etag, Size: r.ContentLength})
}

// HandleListFileUploadParts lists the parts of an upload held here.
func HandleListFileUploadParts(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	parts, err := store.ListParts(p, bucket, key, r.URL.Query().Get("upload_id"))
	if err != nil {
		filesError(w, r, err)
		return
	}
	out := apiclient.FileUploadParts{Parts: make([]apiclient.FileUploadPart, 0, len(parts))}
	for _, pi := range parts {
		out.Parts = append(out.Parts, apiclient.FileUploadPart{Number: pi.Number, ETag: pi.ETag, Size: pi.Size})
	}
	rest.WriteResponse(http.StatusOK, w, r, out)
}

func handleCompleteFileUpload(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	rest.NoDeadlines(w)
	var req apiclient.FileUploadComplete
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	parts := make([]filestore.CompletedPart, 0, len(req.Parts))
	for _, pt := range req.Parts {
		parts = append(parts, filestore.CompletedPart{Number: pt.Number, ETag: pt.ETag})
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	o, err := store.CompleteUpload(p, bucket, key, r.URL.Query().Get("upload_id"), parts, filestore.PutOptions{
		ModifiedBy:  user.Id,
		IfMatch:     r.Header.Get("If-Match"),
		IfNoneMatch: r.Header.Get("If-None-Match") == "*",
	})
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, objectResponse(o))
}

// HandleAbortFileUpload discards an upload and its parts.
func HandleAbortFileUpload(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	bucket, key := filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key")
	if err := store.AbortMultipart(p, bucket, key, r.URL.Query().Get("upload_id")); err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, struct {
		Status bool `json:"status" msgpack:"status"`
	}{true})
}
