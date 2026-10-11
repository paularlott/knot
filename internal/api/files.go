package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
)

// filesContext resolves the store and the caller's principal, writing the
// error response and returning ok=false when the request cannot proceed.
func filesContext(w http.ResponseWriter, r *http.Request) (*filestore.Store, *model.User, *filestore.Principal, bool) {
	store := filestore.Get()
	if store == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: filestore.ErrDisabled.Error()})
		return nil, nil, nil, false
	}
	// A server with no users yet does not authenticate, so there may be no one.
	user, _ := r.Context().Value("user").(*model.User)
	if user == nil {
		rest.WriteResponse(http.StatusUnauthorized, w, r, ErrorResponse{Error: "Authentication required"})
		return nil, nil, nil, false
	}
	p, ok := filestore.PrincipalFor(user)
	if !ok {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "Account is not active"})
		return nil, nil, nil, false
	}
	return store, user, p, true
}

func filesError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, filestore.ErrNoSuchBucket), errors.Is(err, filestore.ErrNoSuchKey):
		status = http.StatusNotFound
	case errors.Is(err, filestore.ErrAccessDenied), errors.Is(err, filestore.ErrCannotOwn), errors.Is(err, filestore.ErrBucketLimit),
		errors.Is(err, filestore.ErrCannotShare), errors.Is(err, filestore.ErrCannotTransfer):
		status = http.StatusForbidden
	case errors.Is(err, filestore.ErrBucketExists), errors.Is(err, filestore.ErrBucketNotEmpty), errors.Is(err, filestore.ErrTransferNameTaken),
		errors.Is(err, filestore.ErrDestinationExists), errors.Is(err, filestore.ErrContentNotHeld):
		status = http.StatusConflict
	case errors.Is(err, filestore.ErrQuotaExceeded):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, filestore.ErrPrecondition):
		status = http.StatusPreconditionFailed
	case errors.Is(err, filestore.ErrInvalidName), errors.Is(err, filestore.ErrInvalidShortName), errors.Is(err, filestore.ErrNameTooLong),
		errors.Is(err, filestore.ErrBucketNamespace), errors.Is(err, filestore.ErrUsernameUnusable), errors.Is(err, filestore.ErrRecipientCannotOwn),
		errors.Is(err, filestore.ErrInvalidKey), errors.Is(err, filestore.ErrInvalidGrant), errors.Is(err, filestore.ErrTooManyGrants),
		errors.Is(err, filestore.ErrMetadataTooLarge), errors.Is(err, filestore.ErrInvalidMetadata), errors.Is(err, filestore.ErrContentMismatch), errors.Is(err, io.ErrUnexpectedEOF),
		errors.Is(err, filestore.ErrInvalidCursor):
		status = http.StatusBadRequest
	case errors.Is(err, filestore.ErrUnavailable):
		status = http.StatusServiceUnavailable
	}
	rest.WriteResponse(status, w, r, ErrorResponse{Error: err.Error()})
}

// nameResolver maps user and group ids to names, caching per request.
type nameResolver struct {
	users  map[string]string
	groups map[string]string
}

func newNameResolver() *nameResolver {
	return &nameResolver{users: map[string]string{}}
}

func (n *nameResolver) user(id string) string {
	if name, ok := n.users[id]; ok {
		return name
	}
	name := id
	if u, err := database.GetInstance().GetUser(id); err == nil && u != nil {
		name = u.Username
	}
	n.users[id] = name
	return name
}

func (n *nameResolver) group(id string) string {
	if n.groups == nil {
		n.groups = map[string]string{}
		if groups, err := database.GetInstance().GetGroups(); err == nil {
			for _, g := range groups {
				n.groups[g.Id] = g.Name
			}
		}
	}
	if name, ok := n.groups[id]; ok {
		return name
	}
	return id
}

func accessName(level int) string {
	switch level {
	case filestore.AccessOwner:
		return "owner"
	case filestore.AccessWrite:
		return "write"
	case filestore.AccessRead:
		return "read"
	}
	return "none"
}

// bucketResponse describes a bucket. Who it is shared with is listed only
// for those who manage it, the owner and file storage managers.
func bucketResponse(b *filestore.BucketInfo, p *filestore.Principal, names *nameResolver) apiclient.FileBucketInfo {
	info := apiclient.FileBucketInfo{
		Id:        b.Id,
		Name:      b.Name,
		Display:   filestore.DisplayName(p, b.Name),
		OwnerId:   b.OwnerId,
		OwnerName: names.user(b.OwnerId),
		Grants:    make([]apiclient.FileGrant, 0, len(b.Grants)),
		Size:      b.Size,
		Count:     b.Count,
		Access:    accessName(b.Access),
		Granted:   accessName(b.Granted),
		Via:       b.Via,
		CreatedAt: b.CreatedAt,
	}
	if b.Access != filestore.AccessOwner {
		return info
	}
	for _, g := range b.Grants {
		fg := apiclient.FileGrant{Type: g.Type, Id: g.Id, Access: g.Access}
		switch g.Type {
		case filestore.GrantUser:
			fg.Name = names.user(g.Id)
		case filestore.GrantGroup:
			fg.Name = names.group(g.Id)
		}
		info.Grants = append(info.Grants, fg)
	}
	return info
}

func writeBucket(w http.ResponseWriter, r *http.Request, store *filestore.Store, p *filestore.Principal, name string, status int) {
	info, err := store.GetBucket(p, name)
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(status, w, r, bucketResponse(info, p, newNameResolver()))
}

func auditBucket(r *http.Request, user *model.User, event, details, bucket string, extra map[string]interface{}) {
	props := map[string]interface{}{"bucket": bucket}
	for k, v := range extra {
		props[k] = v
	}
	audit.LogWithRequest(r, user.Username, model.AuditActorTypeUser, event, details, &props)
}

// RenameUserBuckets renames the buckets a user owns to follow a change of
// username, logging each change.
func RenameUserBuckets(r *http.Request, actor, user *model.User, oldUsername string) {
	store := filestore.Get()
	if store == nil {
		return
	}
	renames, err := store.RenameOwnerBuckets(user.Id, user.Username)
	for _, rn := range renames {
		auditBucket(r, actor, model.AuditEventBucketRename,
			fmt.Sprintf("Renamed bucket %s to %s as %s changed username from %s to %s", rn.OldName, rn.NewName, user.Username, oldUsername, user.Username),
			rn.NewName, map[string]interface{}{"old_name": rn.OldName, "owner_id": user.Id, "owner": user.Username})
	}
	if err != nil {
		log.WithGroup("files").Warn("some buckets could not be renamed with their owner", "user", user.Username, "error", err)
	}
}

// lookupUser finds a user by username, email or id.
func lookupUser(name string) (*model.User, error) {
	db := database.GetInstance()
	if u, err := db.GetUserByUsername(name); err == nil && u != nil && !u.IsDeleted {
		return u, nil
	}
	if u, err := db.GetUserByEmail(name); err == nil && u != nil && !u.IsDeleted {
		return u, nil
	}
	if u, err := db.GetUser(name); err == nil && u != nil && !u.IsDeleted {
		return u, nil
	}
	return nil, fmt.Errorf("user %s not found", name)
}

// lookupGroup finds a group by name or id.
func lookupGroup(name string) (*model.Group, error) {
	groups, err := database.GetInstance().GetGroups()
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		if !g.IsDeleted && (g.Name == name || g.Id == name) {
			return g, nil
		}
	}
	return nil, fmt.Errorf("group %s not found", name)
}

func HandleGetFileBuckets(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	names := newNameResolver()
	list := apiclient.FileBucketList{Buckets: []apiclient.FileBucketInfo{}}
	for _, b := range store.ListBuckets(p, r.URL.Query().Get("all") == "true") {
		list.Buckets = append(list.Buckets, bucketResponse(b, p, names))
	}
	rest.WriteResponse(http.StatusOK, w, r, list)
}

func HandleCreateFileBucket(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	var req apiclient.FileBucketCreateRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	name, err := filestore.NewBucketName(p, strings.TrimSpace(req.Name))
	if err != nil {
		filesError(w, r, err)
		return
	}
	b, err := store.CreateBucket(p, name)
	if err != nil {
		filesError(w, r, err)
		return
	}
	auditBucket(r, user, model.AuditEventBucketCreate, fmt.Sprintf("Created bucket %s", b.Name), b.Name, nil)
	writeBucket(w, r, store, p, b.Name, http.StatusCreated)
}

func HandleGetFileBucket(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	writeBucket(w, r, store, p, filestore.ResolveName(p, r.PathValue("bucket")), http.StatusOK)
}

func HandleDeleteFileBucket(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	name := filestore.ResolveName(p, r.PathValue("bucket"))
	force := r.URL.Query().Get("force") == "true"
	if err := store.DeleteBucket(p, name, force); err != nil {
		filesError(w, r, err)
		return
	}
	auditBucket(r, user, model.AuditEventBucketDelete, fmt.Sprintf("Deleted bucket %s", name), name, map[string]interface{}{"force": force})
	rest.WriteResponse(http.StatusOK, w, r, struct {
		Status bool `json:"status"`
	}{true})
}

// grantPrincipal resolves the principal of a share request to a grant type and id.
func grantPrincipal(grantType, name string) (string, string, string, error) {
	switch grantType {
	case filestore.GrantAll:
		return filestore.GrantAll, "", "all users", nil
	case filestore.GrantUser:
		u, err := lookupUser(name)
		if err != nil {
			return "", "", "", err
		}
		return filestore.GrantUser, u.Id, u.Username, nil
	case filestore.GrantGroup:
		g, err := lookupGroup(name)
		if err != nil {
			return "", "", "", err
		}
		return filestore.GrantGroup, g.Id, g.Name, nil
	}
	return "", "", "", fmt.Errorf("share type must be user, group or all")
}

func HandleShareFileBucket(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	var req apiclient.FileShareRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if req.Access == "" {
		req.Access = filestore.GrantRead
	}
	if req.Access != filestore.GrantRead && req.Access != filestore.GrantWrite {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "access must be read or write"})
		return
	}
	grantType, id, label, err := grantPrincipal(req.Type, req.Name)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	name := filestore.ResolveName(p, r.PathValue("bucket"))
	if _, err := store.SetGrant(p, name, filestore.Grant{Type: grantType, Id: id, Access: req.Access}); err != nil {
		filesError(w, r, err)
		return
	}
	auditBucket(r, user, model.AuditEventBucketShare, fmt.Sprintf("Shared bucket %s with %s (%s)", name, label, req.Access), name, map[string]interface{}{
		"grant_type": grantType, "grant_id": id, "access": req.Access,
	})
	writeBucket(w, r, store, p, name, http.StatusOK)
}

func HandleUnshareFileBucket(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	var req apiclient.FileUnshareRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	grantType, id, label, err := grantPrincipal(req.Type, req.Name)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	name := filestore.ResolveName(p, r.PathValue("bucket"))
	if _, err := store.RemoveGrant(p, name, grantType, id); err != nil {
		filesError(w, r, err)
		return
	}
	auditBucket(r, user, model.AuditEventBucketUnshare, fmt.Sprintf("Stopped sharing bucket %s with %s", name, label), name, map[string]interface{}{
		"grant_type": grantType, "grant_id": id,
	})
	writeBucket(w, r, store, p, name, http.StatusOK)
}

func HandleTransferFileBucket(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	var req apiclient.FileTransferRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	// The bucket first, so nothing is said about the recipient to someone
	// who cannot see the bucket.
	name := filestore.ResolveName(p, r.PathValue("bucket"))
	b, err := store.GetBucket(p, name)
	if err != nil {
		filesError(w, r, err)
		return
	}
	// Who may transfer is settled before anything about the recipient; the
	// store enforces the same rules.
	if b.Access != filestore.AccessOwner {
		filesError(w, r, filestore.ErrAccessDenied)
		return
	}
	if !p.CanTransfer {
		filesError(w, r, filestore.ErrCannotTransfer)
		return
	}
	newOwner, err := lookupUser(req.User)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	// A bucket only goes to someone who can own it; otherwise it would sit
	// with a user who can neither see nor manage it.
	if np, ok := filestore.PrincipalFor(newOwner); !ok || !np.CanOwn {
		filesError(w, r, filestore.ErrRecipientCannotOwn)
		return
	}
	previous := b.OwnerId
	// Only an administrator may push the new owner over their limits.
	nb, err := store.TransferBucket(p, name, newOwner.Id, newOwner.Username, !(req.Force && p.IsAdmin))
	if err != nil {
		filesError(w, r, err)
		return
	}
	auditBucket(r, user, model.AuditEventBucketTransfer, fmt.Sprintf("Transferred bucket %s to %s as %s", name, newOwner.Username, nb.Name), nb.Name, map[string]interface{}{
		"from_user_id": previous, "to_user_id": newOwner.Id, "previous_name": name,
	})
	// The previous owner may no longer see the bucket; describe it anyway.
	info, err := store.Describe(p, nb.Name)
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, bucketResponse(info, p, newNameResolver()))
}

// HandleGetFileShareTargets lists the users and groups a bucket can be
// shared with or transferred to — the share and transfer dialogs' choices,
// without the user management permission /api/users needs. The caller is
// included, marked, so an administrator can transfer a bucket to themselves.
func HandleGetFileShareTargets(w http.ResponseWriter, r *http.Request) {
	_, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	if !p.CanShare && !p.CanTransfer {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: filestore.ErrCannotShare.Error()})
		return
	}

	db := database.GetInstance()
	targets := apiclient.FileShareTargets{Users: []apiclient.FileShareUser{}, Groups: []apiclient.FileShareGroup{}}
	if users, err := db.GetUsers(); err == nil {
		for _, u := range users {
			if u.IsDeleted || !u.Active {
				continue
			}
			up, _ := filestore.PrincipalFor(u)
			targets.Users = append(targets.Users, apiclient.FileShareUser{Id: u.Id, Username: u.Username, CanOwn: up != nil && up.CanOwn, Self: u.Id == user.Id})
		}
	}
	if groups, err := db.GetGroups(); err == nil {
		for _, g := range groups {
			if !g.IsDeleted {
				targets.Groups = append(targets.Groups, apiclient.FileShareGroup{Id: g.Id, Name: g.Name})
			}
		}
	}
	sort.Slice(targets.Users, func(i, j int) bool { return targets.Users[i].Username < targets.Users[j].Username })
	sort.Slice(targets.Groups, func(i, j int) bool { return targets.Groups[i].Name < targets.Groups[j].Name })
	rest.WriteResponse(http.StatusOK, w, r, targets)
}

func HandleGetFileUsage(w http.ResponseWriter, r *http.Request) {
	store, user, _, ok := filesContext(w, r)
	if !ok {
		return
	}

	u := store.Usage(user.Id)
	usage := apiclient.FileUsage{UsedBytes: u.UsedBytes, Objects: u.Objects, Buckets: u.Buckets}
	// The effective limits, including the server defaults when no user or
	// group limit is set.
	if q, err := database.GetUserQuota(user); err == nil {
		usage.QuotaBytes, usage.MaxBuckets = filestore.EffectiveLimits(q)
	}
	rest.WriteResponse(http.StatusOK, w, r, usage)
}

// objectResponse describes an object; its modification time is the file's
// own (the mtime metadata the knot CLI and rclone record) when known, the
// upload time otherwise.
func objectResponse(o *filestore.Object) apiclient.FileObjectInfo {
	modified := o.ModifiedAt
	if t, ok := apiclient.ParseMtime(o.Meta["mtime"]); ok {
		modified = t
	}
	return apiclient.FileObjectInfo{
		Key:         o.Key,
		Size:        o.Size,
		ETag:        o.ETag,
		SHA256:      o.SHA256,
		ContentType: o.ContentType,
		ModifiedAt:  modified,
	}
}

func HandleListFileObjects(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	res, err := store.ListObjects(p, filestore.ResolveName(p, r.PathValue("bucket")), q.Get("prefix"), q.Get("delimiter"), q.Get("after"), limit)
	if err != nil {
		filesError(w, r, err)
		return
	}

	list := apiclient.FileObjectList{
		Objects:     make([]apiclient.FileObjectInfo, 0, len(res.Objects)),
		Prefixes:    res.Prefixes,
		IsTruncated: res.IsTruncated,
		Next:        res.Next,
	}
	if list.Prefixes == nil {
		list.Prefixes = []string{}
	}
	for _, o := range res.Objects {
		list.Objects = append(list.Objects, objectResponse(o))
	}
	rest.WriteResponse(http.StatusOK, w, r, list)
}

// setObjectHeaders writes the headers describing an object's content.
func setObjectHeaders(w http.ResponseWriter, o *filestore.Object) {
	h := w.Header()
	h.Set("Content-Type", o.ContentType)
	// The bytes must arrive as stored: checksums, ETags and ranges depend
	// on it, so proxies must not compress or otherwise transform them.
	h.Set("Cache-Control", "no-transform")
	// A browser that opens a download URL directly saves the file rather
	// than rendering another user's content on the knot origin.
	filestore.SetUntrustedContentHeaders(h)
	h.Set("Content-Disposition", "attachment")
	h.Set("ETag", `"`+o.ETag+`"`)
	h.Set("Last-Modified", o.ModifiedAt.UTC().Format(http.TimeFormat))
	h.Set("X-Knot-Sha256", o.SHA256)
	if mtime, ok := o.Meta["mtime"]; ok {
		h.Set(apiclient.FileMtimeHeader, mtime)
	} else {
		h.Set(apiclient.FileMtimeHeader, apiclient.FormatMtime(o.ModifiedAt))
	}
}

// HandleFilesFsck checks file storage, and with repair set fixes what it
// can. Only file storage administrators may run it.
func HandleFilesFsck(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	if !p.IsAdmin {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: filestore.ErrAccessDenied.Error()})
		return
	}
	rest.NoDeadlines(w)

	var opts filestore.FsckOptions
	if err := rest.DecodeRequestBody(w, r, &opts); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	report, err := store.Fsck(r.Context(), opts)
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, report)
}

func HandleGetFileObject(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	rest.NoDeadlines(w)

	o, f, err := store.OpenObject(r.Context(), p, filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key"))
	if err != nil {
		filesError(w, r, err)
		return
	}
	defer f.Close()

	setObjectHeaders(w, o)
	http.ServeContent(w, r, "", o.ModifiedAt, f)
}

func HandlePutFileObject(w http.ResponseWriter, r *http.Request) {
	store, user, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	rest.NoDeadlines(w)

	opts := filestore.PutOptions{
		ContentType: r.Header.Get("Content-Type"),
		ModifiedBy:  user.Id,
		Size:        r.ContentLength,
		IfMatch:     r.Header.Get("If-Match"),
		IfNoneMatch: r.Header.Get("If-None-Match") == "*",
	}
	if mtime := r.Header.Get(apiclient.FileMtimeHeader); mtime != "" {
		// Kept as metadata, as S3 clients do, so Last-Modified stays the
		// upload time and rclone sees the same modification time.
		if t, ok := apiclient.ParseMtime(mtime); ok {
			opts.Meta = map[string]string{"mtime": apiclient.FormatMtime(t)}
		}
	}

	var o *filestore.Object
	var err error
	if sha := r.Header.Get(apiclient.FileReuseHeader); sha != "" {
		// Content the bucket held until recently: nothing is sent.
		if r.ContentLength > 0 {
			rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "a reuse carries no content"})
			return
		}
		o, err = store.ReuseObject(p, filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key"), strings.ToLower(sha), opts)
	} else {
		o, err = store.PutObject(p, filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key"), r.Body, opts)
	}
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, objectResponse(o))
}

// HandleListFileChanges returns what changed in a bucket since a cursor.
func HandleListFileChanges(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	res, err := store.ListChanges(p, filestore.ResolveName(p, r.PathValue("bucket")), q.Get("prefix"), q.Get("cursor"), limit)
	if err != nil {
		filesError(w, r, err)
		return
	}
	out := apiclient.FileChangeList{
		Changes: make([]apiclient.FileChange, 0, len(res.Changes)),
		Cursor:  res.Cursor,
		More:    res.More,
		Reset:   res.Reset,
	}
	for _, o := range res.Changes {
		if o.IsDeleted {
			out.Changes = append(out.Changes, apiclient.FileChange{FileObjectInfo: apiclient.FileObjectInfo{Key: o.Key, ModifiedAt: o.ModifiedAt}, Deleted: true})
			continue
		}
		out.Changes = append(out.Changes, apiclient.FileChange{FileObjectInfo: objectResponse(o)})
	}
	rest.WriteResponse(http.StatusOK, w, r, out)
}

// HandleCopyFileObject copies a file within or between buckets without
// moving its content: the caller needs read access to the source and write
// access to the destination, and the destination bucket's owner is charged
// for the new file.
func HandleCopyFileObject(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	var req apiclient.FileCopyRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if req.SourceBucket == "" || req.SourceKey == "" || req.DestBucket == "" || req.DestKey == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "source_bucket, source_key, dest_bucket and dest_key are required"})
		return
	}

	src, dst := filestore.ResolveName(p, req.SourceBucket), filestore.ResolveName(p, req.DestBucket)
	if src == dst && req.SourceKey == req.DestKey {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "source and destination are the same file"})
		return
	}

	o, err := store.CopyObjectIf(p, src, req.SourceKey, dst, req.DestKey, nil, filestore.PutOptions{IfMatch: req.IfMatch, IfNoneMatch: req.IfNoneMatch})
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, objectResponse(o))
}

// POST /api/files/move: rename a file, or a folder and its contents, within a
// bucket, on the server.
func HandleMoveFileObjects(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	var req apiclient.FileMoveRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	if req.Bucket == "" || req.From == "" || req.To == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "bucket, from and to are required"})
		return
	}

	n, err := store.MoveObjects(p, filestore.ResolveName(p, req.Bucket), req.From, req.To, req.Overwrite)
	if err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, apiclient.FileMoveResponse{Moved: n})
}

func HandleDeleteFileObject(w http.ResponseWriter, r *http.Request) {
	store, _, p, ok := filesContext(w, r)
	if !ok {
		return
	}

	// With If-Match, only the version the client saw is deleted.
	if err := store.DeleteObjectIf(p, filestore.ResolveName(p, r.PathValue("bucket")), r.PathValue("key"), r.Header.Get("If-Match")); err != nil {
		filesError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, struct {
		Status bool `json:"status"`
	}{true})
}

// fileUsage reports a user's file storage use and effective limits, given
// their summed user and group limits, for the users list and quota views.
func fileUsage(store *filestore.Store, userId string, q *model.Quota) (usedMB float64, buckets int, quotaMB, maxBuckets uint32) {
	u := store.Usage(userId)
	limit, max := filestore.EffectiveLimits(q)
	return float64(u.UsedBytes) / (1024 * 1024), u.Buckets, uint32(limit / (1024 * 1024)), uint32(max)
}
