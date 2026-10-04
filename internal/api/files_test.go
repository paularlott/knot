package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
)

func filesRequest(method, target, body string, user *model.User) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(context.WithValue(req.Context(), "user", user))
}

func TestFilesDisabled(t *testing.T) {
	filestore.SetInstance(nil)
	user := &model.User{Id: "u1", Active: true, Roles: []string{model.RoleAdminUUID}}

	rr := httptest.NewRecorder()
	HandleGetFileBuckets(rr, filesRequest("GET", "/api/files/buckets", "", user))
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "not enabled") {
		t.Errorf("disabled: %d %s", rr.Code, rr.Body.String())
	}
}

func TestFilesHandlers(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()}})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	admin := &model.User{Id: "admin-1", Username: "admin", Active: true, Roles: []string{model.RoleAdminUUID}}
	nobody := &model.User{Id: "user-2", Username: "nobody", Active: true}

	// Users without a file storage permission see only shared buckets and
	// cannot create their own.
	rr := httptest.NewRecorder()
	HandleGetFileBuckets(rr, filesRequest("GET", "/api/files/buckets", "", nobody))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"buckets":[]`) {
		t.Errorf("no permission list: %d %s", rr.Code, rr.Body.String())
	}
	rr = httptest.NewRecorder()
	HandleCreateFileBucket(rr, filesRequest("POST", "/api/files/buckets", `{"name":"nobodys"}`, nobody))
	if rr.Code != http.StatusForbidden {
		t.Errorf("no permission create: %d %s", rr.Code, rr.Body.String())
	}

	// Buckets are created under the caller's username and shown to them by
	// their short name; another user's namespace is refused.
	rr = httptest.NewRecorder()
	HandleCreateFileBucket(rr, filesRequest("POST", "/api/files/buckets", `{"name":"handlers"}`, admin))
	if rr.Code != http.StatusCreated && rr.Code != http.StatusOK {
		t.Fatalf("create: %d %s", rr.Code, rr.Body.String())
	}
	if body := rr.Body.String(); !strings.Contains(body, `"name":"admin--handlers"`) || !strings.Contains(body, `"display_name":"handlers"`) {
		t.Errorf("create response: %s", body)
	}
	for _, bad := range []string{"nobody--handlers", "Bad_Name", "my--bucket--x", "abcdefghijklmnopqrstuvwxyz12345"} {
		rr = httptest.NewRecorder()
		HandleCreateFileBucket(rr, filesRequest("POST", "/api/files/buckets", `{"name":"`+bad+`"}`, admin))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("create %q: %d %s", bad, rr.Code, rr.Body.String())
		}
	}
	if _, err := store.GetBucket(&filestore.Principal{IsAdmin: true}, "admin--handlers"); err != nil {
		t.Fatalf("stored name: %v", err)
	}

	// Upload with a modification time, then download it.
	req := filesRequest("PUT", "/api/files/objects/handlers/dir/a.txt", "hello", admin)
	req.SetPathValue("bucket", "handlers")
	req.SetPathValue("key", "dir/a.txt")
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set(apiclient.FileMtimeHeader, "1600000000.25")
	rr = httptest.NewRecorder()
	HandlePutFileObject(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"key":"dir/a.txt"`) {
		t.Fatalf("put: %d %s", rr.Code, rr.Body.String())
	}

	// The full name refers to the same bucket.
	req = filesRequest("GET", "/api/files/objects/admin--handlers/dir/a.txt", "", admin)
	req.SetPathValue("bucket", "admin--handlers")
	req.SetPathValue("key", "dir/a.txt")
	rr = httptest.NewRecorder()
	HandleGetFileObject(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "hello" || rr.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("get: %d %q %v", rr.Code, rr.Body.String(), rr.Header())
	}
	if rr.Header().Get("Cache-Control") != "no-transform" {
		t.Errorf("downloads must forbid proxy transforms: %v", rr.Header())
	}
	// Another user's content must never render, or run script, on the origin.
	if h := rr.Header(); h.Get("Content-Disposition") != "attachment" || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.HasPrefix(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("untrusted content headers missing: %v", h)
	}
	if mtime, _ := apiclient.ParseMtime(rr.Header().Get(apiclient.FileMtimeHeader)); mtime.Unix() != 1600000000 {
		t.Errorf("mtime header %q", rr.Header().Get(apiclient.FileMtimeHeader))
	}

	// Listing reports the file's own modification time.
	req = filesRequest("GET", "/api/files/list/handlers?delimiter=/", "", admin)
	req.SetPathValue("bucket", "handlers")
	rr = httptest.NewRecorder()
	HandleListFileObjects(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"prefixes":["dir/"]`) {
		t.Errorf("list: %d %s", rr.Code, rr.Body.String())
	}

	// Errors map to HTTP status codes.
	req = filesRequest("GET", "/api/files/objects/handlers/missing", "", admin)
	req.SetPathValue("bucket", "handlers")
	req.SetPathValue("key", "missing")
	rr = httptest.NewRecorder()
	HandleGetFileObject(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("missing object: %d", rr.Code)
	}

	req = filesRequest("DELETE", "/api/files/buckets/handlers", "", admin)
	req.SetPathValue("bucket", "handlers")
	rr = httptest.NewRecorder()
	HandleDeleteFileBucket(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("delete non-empty bucket: %d %s", rr.Code, rr.Body.String())
	}

	req = filesRequest("DELETE", "/api/files/objects/handlers/dir/a.txt", "", admin)
	req.SetPathValue("bucket", "handlers")
	req.SetPathValue("key", "dir/a.txt")
	rr = httptest.NewRecorder()
	HandleDeleteFileObject(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("delete object: %d %s", rr.Code, rr.Body.String())
	}
}

// A bucket can only be transferred to someone who may own buckets, and an
// administrator can take a bucket back themselves.
func TestFileTransferRecipients(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-files-transfer",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	store, err := filestore.Open(filestore.Config{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	// Use File Storage without share or transfer.
	role := model.NewRole("files-only-"+t.Name(), []uint16{model.PermissionUseFiles}, "")
	model.SaveRoleToCache(role)

	db := database.GetInstance()
	admin := &model.User{Id: "ft-admin", Username: "ftadmin", Email: "ftadmin@test.local", Active: true, Roles: []string{model.RoleAdminUUID}}
	owner := &model.User{Id: "ft-owner", Username: "ftowner", Email: "ftowner@test.local", Active: true, Roles: []string{role.Id}}
	plain := &model.User{Id: "ft-plain", Username: "ftplain", Email: "ftplain@test.local", Active: true}
	for _, u := range []*model.User{admin, owner, plain} {
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatalf("SaveUser %s: %v", u.Username, err)
		}
	}

	ap, _ := filestore.PrincipalFor(admin)
	if _, err := store.CreateBucket(ap, "ftadmin--stuff"); err != nil {
		t.Fatal(err)
	}
	transfer := func(bucket, to string) *httptest.ResponseRecorder {
		req := filesRequest("POST", "/api/files/buckets/"+bucket+"/transfer", `{"user":"`+to+`"}`, admin)
		req.SetPathValue("bucket", bucket)
		rr := httptest.NewRecorder()
		HandleTransferFileBucket(rr, req)
		return rr
	}

	// No file storage permission: refused, and the bucket stays put.
	if rr := transfer("stuff", "ftplain"); rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "permission to use file storage") {
		t.Errorf("transfer to user without permission: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := store.GetBucket(ap, "ftadmin--stuff"); err != nil {
		t.Errorf("bucket moved: %v", err)
	}

	if rr := transfer("stuff", "ftowner"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"name":"ftowner--stuff"`) {
		t.Fatalf("transfer to owner: %d %s", rr.Code, rr.Body.String())
	}
	// And back to the administrator.
	if rr := transfer("ftowner--stuff", "ftadmin"); rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"name":"ftadmin--stuff"`) {
		t.Fatalf("transfer back to admin: %d %s", rr.Code, rr.Body.String())
	}

	// The transfer dialog's choices: the caller included and marked, and
	// who may receive a bucket.
	rr := httptest.NewRecorder()
	HandleGetFileShareTargets(rr, filesRequest("GET", "/api/files/share-targets", "", admin))
	var targets apiclient.FileShareTargets
	if err := json.Unmarshal(rr.Body.Bytes(), &targets); err != nil {
		t.Fatalf("targets: %d %s", rr.Code, rr.Body.String())
	}
	got := map[string]apiclient.FileShareUser{}
	for _, u := range targets.Users {
		got[u.Username] = u
	}
	if u := got["ftadmin"]; !u.Self || !u.CanOwn {
		t.Errorf("admin target %+v", u)
	}
	if u := got["ftowner"]; u.Self || !u.CanOwn {
		t.Errorf("owner target %+v", u)
	}
	if u, ok := got["ftplain"]; !ok || u.CanOwn {
		t.Errorf("plain target %+v %v", u, ok)
	}
}

// Only a file storage administrator may force a transfer past the new
// owner's quota; force from anyone else is ignored.
func TestFileTransferForceAdminOnly(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-files-force",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	// The recipient has room for a single byte; everyone else is unlimited.
	store, err := filestore.Open(filestore.Config{Dir: t.TempDir(), Quota: func(id string) (int64, error) {
		if id == "ff-other" {
			return 1, nil
		}
		return 0, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	role := model.NewRole("files-transfer-"+t.Name(), []uint16{model.PermissionUseFiles, model.PermissionTransferBuckets}, "")
	model.SaveRoleToCache(role)
	db := database.GetInstance()
	admin := &model.User{Id: "ff-admin", Username: "ffadmin", Email: "ffadmin@test.local", Active: true, Roles: []string{model.RoleAdminUUID}}
	owner := &model.User{Id: "ff-owner", Username: "ffowner", Email: "ffowner@test.local", Active: true, Roles: []string{role.Id}}
	other := &model.User{Id: "ff-other", Username: "ffother", Email: "ffother@test.local", Active: true, Roles: []string{role.Id}}
	for _, u := range []*model.User{admin, owner, other} {
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatalf("SaveUser %s: %v", u.Username, err)
		}
	}

	op, _ := filestore.PrincipalFor(owner)
	if _, err := store.CreateBucket(op, "ffowner--big"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.PutObject(op, "ffowner--big", "f", strings.NewReader("12345"), filestore.PutOptions{Size: 5}); err != nil {
		t.Fatal(err)
	}

	transfer := func(as *model.User, bucket, to string) *httptest.ResponseRecorder {
		req := filesRequest("POST", "/api/files/buckets/"+bucket+"/transfer", `{"user":"`+to+`","force":true}`, as)
		req.SetPathValue("bucket", bucket)
		rr := httptest.NewRecorder()
		HandleTransferFileBucket(rr, req)
		return rr
	}
	if rr := transfer(owner, "big", "ffother"); rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("owner forced transfer over quota: %d %s", rr.Code, rr.Body.String())
	}
	if rr := transfer(admin, "ffowner--big", "ffother"); rr.Code != http.StatusOK {
		t.Errorf("admin forced transfer: %d %s", rr.Code, rr.Body.String())
	}
}

func TestFileCopyHandler(t *testing.T) {
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-files-copy",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	store, err := filestore.Open(filestore.Config{Dir: t.TempDir(), Quota: func(id string) (int64, error) {
		if id == "fc-small" {
			return 10, nil
		}
		return 0, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	role := model.NewRole("files-copy-"+t.Name(), []uint16{model.PermissionUseFiles, model.PermissionShareBuckets}, "")
	model.SaveRoleToCache(role)
	db := database.GetInstance()
	mk := func(id string) *model.User {
		u := &model.User{Id: id, Username: id[3:], Email: id[3:] + "@test.local", Active: true, Roles: []string{role.Id}}
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatal(err)
		}
		return u
	}
	alice, bob, small := mk("fc-alice"), mk("fc-bob"), mk("fc-small")
	ap, _ := filestore.PrincipalFor(alice)
	bp, _ := filestore.PrincipalFor(bob)
	sp, _ := filestore.PrincipalFor(small)

	for _, c := range []struct {
		p    *filestore.Principal
		name string
	}{{ap, "alice--src"}, {ap, "alice--dst"}, {bp, "bob--private"}, {sp, "small--tiny"}} {
		if _, err := store.CreateBucket(c.p, c.name); err != nil {
			t.Fatal(err)
		}
	}
	put := func(p *filestore.Principal, bucket, key, content string, opts filestore.PutOptions) *filestore.Object {
		opts.Size = int64(len(content))
		o, err := store.PutObject(p, bucket, key, strings.NewReader(content), opts)
		if err != nil {
			t.Fatalf("put %s/%s: %v", bucket, key, err)
		}
		return o
	}
	put(ap, "alice--src", "a.txt", "hello copy", filestore.PutOptions{ContentType: "text/x-test", Meta: map[string]string{"mtime": "1600000000.5", "note": "kept"}})
	put(ap, "alice--src", "big.bin", strings.Repeat("x", 50), filestore.PutOptions{})
	put(bp, "bob--private", "secret.txt", "bob's", filestore.PutOptions{})

	copyReq := func(user *model.User, body string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		HandleCopyFileObject(rr, filesRequest("POST", "/api/files/copy", body, user))
		return rr
	}
	body := func(sb, sk, db, dk string) string {
		b, _ := json.Marshal(apiclient.FileCopyRequest{SourceBucket: sb, SourceKey: sk, DestBucket: db, DestKey: dk})
		return string(b)
	}
	read := func(p *filestore.Principal, bucket, key string) (*filestore.Object, string) {
		o, f, err := store.OpenObject(context.Background(), p, bucket, key)
		if err != nil {
			t.Fatalf("open %s/%s: %v", bucket, key, err)
		}
		defer f.Close()
		data := make([]byte, o.Size)
		f.Read(data)
		return o, string(data)
	}

	// Between buckets, by short names: content, type, metadata and the
	// modification time all come across, and the response describes the copy.
	rr := copyReq(alice, body("src", "a.txt", "dst", "dir/copy.txt"))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), `"key":"dir/copy.txt"`) {
		t.Fatalf("copy between buckets: %d %s", rr.Code, rr.Body.String())
	}
	o, content := read(ap, "alice--dst", "dir/copy.txt")
	if content != "hello copy" || o.ContentType != "text/x-test" || o.Meta["note"] != "kept" || o.Meta["mtime"] != "1600000000.5" {
		t.Errorf("copy lost data: %q %+v", content, o)
	}
	if !strings.Contains(rr.Body.String(), "2020-09-13T") {
		t.Errorf("response does not carry the original modification time: %s", rr.Body.String())
	}

	// Full names refer to the same buckets; within one bucket; overwriting.
	if rr := copyReq(alice, body("alice--src", "a.txt", "alice--src", "b.txt")); rr.Code != http.StatusOK {
		t.Errorf("copy within a bucket: %d %s", rr.Code, rr.Body.String())
	}
	put(ap, "alice--dst", "over.txt", "old", filestore.PutOptions{})
	if rr := copyReq(alice, body("src", "a.txt", "dst", "over.txt")); rr.Code != http.StatusOK {
		t.Errorf("copy over an existing file: %d %s", rr.Code, rr.Body.String())
	}
	if _, c := read(ap, "alice--dst", "over.txt"); c != "hello copy" {
		t.Errorf("destination not replaced: %q", c)
	}

	// Content is shared, not duplicated: deleting the source leaves the copy.
	if err := store.DeleteObject(ap, "alice--src", "a.txt"); err != nil {
		t.Fatal(err)
	}
	if _, c := read(ap, "alice--dst", "dir/copy.txt"); c != "hello copy" {
		t.Errorf("copy lost when the source went: %q", c)
	}

	// The same file is refused, in short or full names.
	for _, b := range []string{body("src", "b.txt", "src", "b.txt"), body("src", "b.txt", "alice--src", "b.txt")} {
		if rr := copyReq(alice, b); rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "same file") {
			t.Errorf("copy onto itself: %d %s", rr.Code, rr.Body.String())
		}
	}

	// Missing source, missing fields, bad destination key and bad JSON.
	if rr := copyReq(alice, body("src", "nope.txt", "dst", "x")); rr.Code != http.StatusNotFound {
		t.Errorf("missing source file: %d %s", rr.Code, rr.Body.String())
	}
	if rr := copyReq(alice, body("nobucket", "x", "dst", "x")); rr.Code != http.StatusNotFound {
		t.Errorf("missing source bucket: %d %s", rr.Code, rr.Body.String())
	}
	if rr := copyReq(alice, body("src", "b.txt", "nobucket", "x")); rr.Code != http.StatusNotFound {
		t.Errorf("missing destination bucket: %d %s", rr.Code, rr.Body.String())
	}
	for _, b := range []string{body("", "a", "dst", "x"), body("src", "", "dst", "x"), body("src", "a", "", "x"), body("src", "a", "dst", ""), `{}`} {
		if rr := copyReq(alice, b); rr.Code != http.StatusBadRequest {
			t.Errorf("missing field %s: %d %s", b, rr.Code, rr.Body.String())
		}
	}
	if rr := copyReq(alice, body("src", "b.txt", "dst", "a/../b")); rr.Code != http.StatusBadRequest {
		t.Errorf("invalid destination key: %d %s", rr.Code, rr.Body.String())
	}
	if rr := copyReq(alice, `not json`); rr.Code != http.StatusBadRequest {
		t.Errorf("bad JSON: %d %s", rr.Code, rr.Body.String())
	}

	// Another user's private bucket can be neither read from nor written to.
	if rr := copyReq(alice, body("bob--private", "secret.txt", "dst", "stolen.txt")); rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Errorf("copy from a private bucket: %d %s", rr.Code, rr.Body.String())
	}
	if rr := copyReq(alice, body("src", "b.txt", "bob--private", "planted.txt")); rr.Code != http.StatusNotFound && rr.Code != http.StatusForbidden {
		t.Errorf("copy into a private bucket: %d %s", rr.Code, rr.Body.String())
	}
	if _, _, err := store.OpenObject(context.Background(), bp, "bob--private", "planted.txt"); err == nil {
		t.Error("a file was planted in a private bucket")
	}

	// Read access is enough for a source, write for a destination.
	if _, err := store.SetGrant(bp, "bob--private", filestore.Grant{Type: filestore.GrantUser, Id: alice.Id, Access: filestore.GrantRead}); err != nil {
		t.Fatal(err)
	}
	if rr := copyReq(alice, body("bob--private", "secret.txt", "dst", "from-bob.txt")); rr.Code != http.StatusOK {
		t.Errorf("copy from a bucket shared read-only: %d %s", rr.Code, rr.Body.String())
	}
	if rr := copyReq(alice, body("src", "b.txt", "bob--private", "planted.txt")); rr.Code != http.StatusForbidden {
		t.Errorf("copy into a bucket shared read-only: %d %s", rr.Code, rr.Body.String())
	}
	if _, err := store.SetGrant(bp, "bob--private", filestore.Grant{Type: filestore.GrantUser, Id: alice.Id, Access: filestore.GrantWrite}); err != nil {
		t.Fatal(err)
	}
	if rr := copyReq(alice, body("src", "b.txt", "bob--private", "planted.txt")); rr.Code != http.StatusOK {
		t.Errorf("copy into a bucket shared read-write: %d %s", rr.Code, rr.Body.String())
	}

	// A copy is charged to the destination bucket's owner, so a full owner
	// refuses it, and nothing is stored.
	if _, err := store.SetGrant(sp, "small--tiny", filestore.Grant{Type: filestore.GrantUser, Id: alice.Id, Access: filestore.GrantWrite}); err != nil {
		t.Fatal(err)
	}
	if rr := copyReq(alice, body("src", "big.bin", "small--tiny", "big.bin")); rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("copy over the owner's quota: %d %s", rr.Code, rr.Body.String())
	}
	if _, _, err := store.OpenObject(context.Background(), sp, "small--tiny", "big.bin"); err == nil {
		t.Error("an over-quota copy was stored")
	}
}
