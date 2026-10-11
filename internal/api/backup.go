package api

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
)

// Backup and restore are streams of JSON lines, one stream for each kind of
// record, so a server of any size is backed up and restored without holding
// it in memory, by a client that can run anywhere. Backing up needs the
// Backup Server permission. A restore needs it too, except on a server with
// no users yet, which is how a lost server is rebuilt.

var backupKinds = apiclient.BackupKinds

func isFileKind(kind string) bool { return kind == "file-buckets" || kind == "file-objects" }

func validBackupKind(kind string) bool {
	for _, k := range backupKinds {
		if k == kind {
			return true
		}
	}
	return false
}

// backupActor names who is acting for the audit log.
func backupActor(r *http.Request) string {
	if user, _ := r.Context().Value("user").(*model.User); user != nil {
		return user.Username
	}
	return "setup"
}

func HandleGetBackupInfo(w http.ResponseWriter, r *http.Request) {
	kinds := make([]string, 0, len(backupKinds))
	files := filestore.Get() != nil
	for _, k := range backupKinds {
		if isFileKind(k) && !files {
			continue
		}
		kinds = append(kinds, k)
	}
	audit.LogWithRequest(r, backupActor(r), model.AuditActorTypeUser, model.AuditEventBackup, "Backup started", nil)
	rest.WriteResponse(http.StatusOK, w, r, apiclient.BackupInfo{Version: build.Version, Time: time.Now().UTC(), Kinds: kinds, Files: files})
}

func summaryDetails(what string, s *apiclient.BackupSummary) (string, *map[string]interface{}) {
	total := 0
	for _, n := range s.Counts {
		total += n
	}
	return fmt.Sprintf("%s: %d records, %d warnings", what, total, s.Warnings), &map[string]interface{}{"counts": s.Counts, "warnings": s.Warnings}
}

func HandleBackupComplete(w http.ResponseWriter, r *http.Request) {
	var s apiclient.BackupSummary
	if err := rest.DecodeRequestBody(w, r, &s); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	details, props := summaryDetails("Backup completed", &s)
	audit.LogWithRequest(r, backupActor(r), model.AuditActorTypeUser, model.AuditEventBackup, details, props)
	w.WriteHeader(http.StatusOK)
}

func HandleRestoreComplete(w http.ResponseWriter, r *http.Request) {
	var s apiclient.BackupSummary
	if err := rest.DecodeRequestBody(w, r, &s); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	details, props := summaryDetails("Restore completed", &s)
	audit.LogWithRequest(r, backupActor(r), model.AuditActorTypeUser, model.AuditEventRestore, details, props)
	w.WriteHeader(http.StatusOK)
}

// lineWriter writes JSON lines, flushing now and then so a client sees them
// as they come.
type lineWriter struct {
	w     http.ResponseWriter
	bw    *bufio.Writer
	enc   *json.Encoder
	flush http.Flusher
	n     int
}

func newLineWriter(w http.ResponseWriter) *lineWriter {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	bw := bufio.NewWriterSize(w, 64*1024)
	lw := &lineWriter{w: w, bw: bw, enc: json.NewEncoder(bw)}
	lw.flush, _ = w.(http.Flusher)
	return lw
}

func (lw *lineWriter) emit(v any) error {
	if err := lw.enc.Encode(v); err != nil {
		return err
	}
	lw.n++
	if lw.n%1000 == 0 {
		if err := lw.bw.Flush(); err != nil {
			return err
		}
		if lw.flush != nil {
			lw.flush.Flush()
		}
	}
	return nil
}

// close ends the stream with a line giving how many records it held, which
// a client that reads it knows it has them all: a stream cut short, even at
// a record boundary, lacks it.
func (lw *lineWriter) close() error {
	if err := lw.enc.Encode(map[string]int{apiclient.BackupEndKey: lw.n}); err != nil {
		return err
	}
	return lw.bw.Flush()
}

// GET /api/backup/{kind}: every record of a kind, one JSON line each.
// limit_user and limit_template narrow the backup to one user or template.
func HandleBackupKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !validBackupKind(kind) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Unknown kind of record " + kind})
		return
	}
	if isFileKind(kind) && filestore.Get() == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return
	}
	limitUser, limitTemplate := r.URL.Query().Get("limit_user"), r.URL.Query().Get("limit_template")

	// Anything that can fail before the stream starts does so here, so the
	// client gets an error status rather than a stream that ends early.
	var ownerId string
	if limitUser != "" {
		user, err := database.GetInstance().GetUserByUsername(limitUser)
		if err != nil {
			rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "No such user " + limitUser})
			return
		}
		ownerId = user.Id
	}

	rest.NoDeadlines(w)
	lw := newLineWriter(w)
	err := streamKind(r.Context(), kind, limitUser, ownerId, limitTemplate, lw.emit)
	if err == nil {
		err = lw.close()
	}
	if err != nil {
		// The status is sent; the client sees a stream without its end
		// line and fails the backup.
		panic(http.ErrAbortHandler)
	}
}

// streamKind emits the records of a kind.
func streamKind(ctx context.Context, kind, limitUser, ownerId, limitTemplate string, emit func(any) error) error {
	db := database.GetInstance()

	each := func(n int, get func(i int) any) error {
		for i := 0; i < n; i++ {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := emit(get(i)); err != nil {
				return err
			}
		}
		return nil
	}

	switch kind {
	case "audit-logs":
		v, _, err := db.GetAuditLogs(nil, 0, 0)
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "cfg-values":
		v, err := db.GetCfgValues()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "templates":
		v, err := db.GetTemplates()
		if err != nil {
			return err
		}
		for _, t := range v {
			if limitTemplate == "" || t.Name == limitTemplate {
				if err := emit(t); err != nil {
					return err
				}
			}
		}
		return nil
	case "template-vars":
		v, err := db.GetTemplateVars()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "volumes":
		v, err := db.GetVolumes()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "groups":
		v, err := db.GetGroups()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "roles":
		v, err := db.GetRoles()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "scripts":
		v, err := db.GetScripts()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "skills":
		v, err := db.GetSkills()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "commands":
		v, err := db.GetCommands()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "responses":
		v, err := db.GetResponses()
		if err != nil {
			return err
		}
		return each(len(v), func(i int) any { return v[i] })
	case "users", "tokens", "spaces":
		users, err := db.GetUsers()
		if err != nil {
			return err
		}
		for _, u := range users {
			if limitUser != "" && u.Username != limitUser {
				continue
			}
			switch kind {
			case "users":
				if err := emit(u); err != nil {
					return err
				}
			case "tokens":
				tokens, err := db.GetTokensForUser(u.Id)
				if err != nil {
					return err
				}
				for _, t := range tokens {
					if err := emit(t); err != nil {
						return err
					}
				}
			case "spaces":
				spaces, err := db.GetSpacesForUser(u.Id)
				if err != nil {
					return err
				}
				for _, s := range spaces {
					space, err := db.GetSpace(s.Id)
					if err != nil {
						return err
					}
					if err := emit(space); err != nil {
						return err
					}
				}
			}
		}
		return nil
	case "file-buckets", "file-objects":
		store := filestore.Get()
		var keep func(*filestore.Bucket) bool
		if limitUser != "" {
			keep = func(b *filestore.Bucket) bool { return b.OwnerId == ownerId }
		}
		if kind == "file-buckets" {
			return store.StreamBuckets(keep, func(b *filestore.Bucket) error { return emit(b) })
		}
		return store.StreamObjects(keep, func(o *filestore.Object) error { return emit(o) })
	}
	return fmt.Errorf("unknown kind %s", kind)
}

// GET /api/backup/content/{sha}: a file's content by its checksum, fetched
// from another server if this one does not hold it.
func HandleBackupContent(w http.ResponseWriter, r *http.Request) {
	store := filestore.Get()
	if store == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return
	}
	if !filestore.ValidSHA(r.PathValue("sha")) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid checksum"})
		return
	}
	rest.NoDeadlines(w)
	c, err := store.ReadContent(r.Context(), r.PathValue("sha"))
	if err != nil {
		filesError(w, r, err)
		return
	}
	defer c.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "", time.Time{}, c)
}

// restoreBatch is how many file records are saved at a time.
const restoreBatch = 500

// maxRestoreErrors is how many failures a restore reports in full.
const maxRestoreErrors = 20

// POST /api/restore/{kind}: save the records of a kind, sent as JSON lines.
func HandleRestoreKind(w http.ResponseWriter, r *http.Request) {
	kind := r.PathValue("kind")
	if !validBackupKind(kind) {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Unknown kind of record " + kind})
		return
	}
	if isFileKind(kind) && filestore.Get() == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return
	}
	rest.NoDeadlines(w)
	if err := rest.GunzipRequest(r); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid compressed body: " + err.Error()})
		return
	}

	result := apiclient.RestoreResult{}
	fail := func(err error, n int) {
		result.Skipped++
		if len(result.Errors) < maxRestoreErrors {
			result.Errors = append(result.Errors, fmt.Sprintf("record %d: %v", n, err))
		}
	}

	save, flush := restoreSaver(kind, &result, fail)
	dec := json.NewDecoder(r.Body)
	n := 0
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: fmt.Sprintf("record %d: %v", n+1, err)})
			return
		}
		n++
		save(raw, n)
	}
	flush()

	rest.WriteResponse(http.StatusOK, w, r, result)
}

// restoreSaver returns a function that saves one record of a kind, and one
// that finishes any batch it holds.
func restoreSaver(kind string, result *apiclient.RestoreResult, fail func(error, int)) (func(json.RawMessage, int), func()) {
	db := database.GetInstance()

	// plain decodes a record into a model and saves it.
	plain := func(into func() any, save func(v any) error) (func(json.RawMessage, int), func()) {
		return func(raw json.RawMessage, n int) {
			v := into()
			if err := json.Unmarshal(raw, v); err != nil {
				fail(err, n)
				return
			}
			if err := save(v); err != nil {
				fail(err, n)
				return
			}
			result.Restored++
		}, func() {}
	}

	switch kind {
	case "audit-logs":
		return plain(func() any { return &model.AuditLogEntry{} }, func(v any) error {
			entry := v.(*model.AuditLogEntry)
			// The numbers are this server's own, and the restored server has
			// entries of its own; the database gives the restored ones new ones.
			entry.Id = 0
			return db.SaveAuditLog(entry)
		})
	case "cfg-values":
		return plain(func() any { return &model.CfgValue{} }, func(v any) error { return db.SaveCfgValue(v.(*model.CfgValue)) })
	case "templates":
		return plain(func() any { return &model.Template{} }, func(v any) error { return db.SaveTemplate(v.(*model.Template), nil) })
	case "template-vars":
		return plain(func() any { return &model.TemplateVar{} }, func(v any) error { return db.SaveTemplateVar(v.(*model.TemplateVar)) })
	case "volumes":
		return plain(func() any { return &model.Volume{} }, func(v any) error { return db.SaveVolume(v.(*model.Volume), nil) })
	case "groups":
		return plain(func() any { return &model.Group{} }, func(v any) error { return db.SaveGroup(v.(*model.Group)) })
	case "roles":
		return plain(func() any { return &model.Role{} }, func(v any) error {
			role := v.(*model.Role)
			// The built-in roles are not kept in the database.
			if model.IsBuiltinRole(role.Id) {
				return nil
			}
			if err := db.SaveRole(role); err != nil {
				return err
			}
			if role.IsDeleted {
				model.DeleteRoleFromCache(role.Id)
			} else {
				model.SaveRoleToCache(role)
			}
			return nil
		})
	case "scripts":
		return plain(func() any { return &model.Script{} }, func(v any) error { return db.SaveScript(v.(*model.Script), nil) })
	case "skills":
		return plain(func() any { return &model.Skill{} }, func(v any) error { return db.SaveSkill(v.(*model.Skill), nil) })
	case "commands":
		return plain(func() any { return &model.Command{} }, func(v any) error { return db.SaveCommand(v.(*model.Command), nil) })
	case "responses":
		return plain(func() any { return &model.Response{} }, func(v any) error { return db.SaveResponse(v.(*model.Response)) })
	case "tokens":
		return plain(func() any { return &model.Token{} }, func(v any) error { return db.SaveToken(v.(*model.Token)) })
	case "users":
		return plain(func() any { return &model.User{} }, func(v any) error { return db.SaveUser(v.(*model.User), nil) })
	case "spaces":
		return plain(func() any { return &model.Space{} }, func(v any) error {
			space := v.(*model.Space)
			// A space that never started is given a start time.
			if space.StartedAt.IsZero() {
				space.StartedAt = time.Now().UTC()
			}
			return db.SaveSpace(space, nil)
		})
	case "file-buckets":
		store := filestore.Get()
		var batch []*filestore.Bucket
		last := 0
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if _, err := store.RestoreBuckets(batch); err != nil {
				// None of the batch counts as restored.
				result.Skipped += len(batch)
				if len(result.Errors) < maxRestoreErrors {
					result.Errors = append(result.Errors, fmt.Sprintf("buckets before record %d: %v", last+1, err))
				}
			} else {
				result.Restored += len(batch)
			}
			batch = batch[:0]
		}
		return func(raw json.RawMessage, n int) {
			b := &filestore.Bucket{}
			if err := json.Unmarshal(raw, b); err != nil || !filestore.ValidBucketId(b.Id) || !filestore.ValidBucketName(b.Name) {
				fail(fmt.Errorf("not a bucket record"), n)
				return
			}
			batch = append(batch, b)
			last = n
			if len(batch) >= restoreBatch {
				flush()
			}
		}, flush
	case "file-objects":
		store := filestore.Get()
		var batch []*filestore.Object
		last := 0
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if _, err := store.RestoreObjects(batch); err != nil {
				// None of the batch counts as restored.
				result.Skipped += len(batch)
				if len(result.Errors) < maxRestoreErrors {
					result.Errors = append(result.Errors, fmt.Sprintf("files before record %d: %v", last+1, err))
				}
			} else {
				result.Restored += len(batch)
			}
			batch = batch[:0]
		}
		return func(raw json.RawMessage, n int) {
			o := &filestore.Object{}
			if err := json.Unmarshal(raw, o); err != nil || !filestore.ValidBucketId(o.BucketId) || !filestore.ValidKey(o.Key) {
				fail(fmt.Errorf("not a file record"), n)
				return
			}
			batch = append(batch, o)
			last = n
			if len(batch) >= restoreBatch {
				flush()
			}
		}, flush
	}
	return func(json.RawMessage, int) {}, func() {}
}

// POST /api/restore/content/missing: which of some checksums this server
// holds no content for, so a restore sends only what is needed.
func HandleRestoreContentMissing(w http.ResponseWriter, r *http.Request) {
	store := filestore.Get()
	if store == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return
	}
	var req struct {
		Shas []string `json:"shas"`
	}
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	missing := store.MissingContent(req.Shas)
	if missing == nil {
		missing = []string{}
	}
	rest.WriteResponse(http.StatusOK, w, r, struct {
		Missing []string `json:"missing"`
	}{missing})
}

// PUT /api/restore/content/{sha}: store a file's content, checked against its
// checksum. Content no file refers to is not kept.
func HandleRestoreContent(w http.ResponseWriter, r *http.Request) {
	store := filestore.Get()
	if store == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return
	}
	sha := r.PathValue("sha")
	if !filestore.ValidSHA(sha) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid checksum"})
		return
	}
	rest.NoDeadlines(w)
	if err := store.ImportContent(r.Body, sha); err != nil {
		filesError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
