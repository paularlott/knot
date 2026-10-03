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
