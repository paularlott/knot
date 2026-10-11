package command_files

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/api"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
)

// testDBDir holds the database the handlers look owners up in, shared by
// every test as the database driver is opened once per process.
var testDBDir string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "knot-files-cli-db")
	if err != nil {
		panic(err)
	}
	testDBDir = dir
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// adminUser is who the in-process server authenticates every request as.
var adminUser = &model.User{Id: "u1", Username: "tester", Active: true, Roles: []string{model.RoleAdminUUID}}

// newFilesServer runs the files API in-process against a temporary store,
// authenticated as an administrator.
func newFilesServer(t *testing.T) (*apiclient.ApiClient, *filestore.Store) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{BadgerDB: config.BadgerDBConfig{Enabled: true, Path: testDBDir}})
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

	user := adminUser
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/buckets/{bucket}", api.HandleGetFileBucket)
	mux.HandleFunc("POST /api/files/copy", api.HandleCopyFileObject)
	mux.HandleFunc("POST /api/files/move", api.HandleMoveFileObjects)
	mux.HandleFunc("GET /api/files/list/{bucket}", api.HandleListFileObjects)
	mux.HandleFunc("GET /api/files/changes/{bucket}", api.HandleListFileChanges)
	mux.HandleFunc("GET /api/files/objects/{bucket}/{key...}", api.HandleGetFileObject)
	mux.HandleFunc("PUT /api/files/objects/{bucket}/{key...}", api.HandlePutFileObject)
	mux.HandleFunc("DELETE /api/files/objects/{bucket}/{key...}", api.HandleDeleteFileObject)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "user", user)))
	}))
	t.Cleanup(srv.Close)

	p, _ := filestore.PrincipalFor(user)
	if _, err := store.CreateBucket(p, "tester--sync-test"); err != nil {
		t.Fatal(err)
	}

	client, err := apiclient.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	return client, store
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0755)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func remoteKeys(t *testing.T, client *apiclient.ApiClient) []string {
	t.Helper()
	objs, _, err := listAll(context.Background(), client, "sync-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, o := range objs {
		keys = append(keys, o.Key)
	}
	sort.Strings(keys)
	return keys
}

func TestSyncUpAndDown(t *testing.T) {
	client, store := newFilesServer(t)
	ctx := context.Background()
	src := t.TempDir()

	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "beta")
	old := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	os.Chtimes(filepath.Join(src, "a.txt"), old, old)

	// A dry run changes nothing.
	if err := syncUp(ctx, client, src, "sync-test", "cfg/", false, true); err != nil {
		t.Fatal(err)
	}
	if keys := remoteKeys(t, client); len(keys) != 0 {
		t.Fatalf("dry run uploaded %v", keys)
	}

	if err := syncUp(ctx, client, src, "sync-test", "cfg/", false, false); err != nil {
		t.Fatal(err)
	}
	if keys := remoteKeys(t, client); len(keys) != 2 || keys[0] != "cfg/a.txt" || keys[1] != "cfg/sub/b.txt" {
		t.Fatalf("after first sync %v", keys)
	}

	// A file that already matches is not uploaded again: change one and
	// check only it was rewritten. Any write, even of identical content,
	// moves the record's update time.
	admin := &filestore.Principal{IsAdmin: true}
	record := func(key string) *filestore.Object {
		o, err := store.HeadObject(admin, "tester--sync-test", key)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	aBefore, bBefore := record("cfg/a.txt"), record("cfg/sub/b.txt")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "beta v2")
	if err := syncUp(ctx, client, src, "sync-test", "cfg/", false, false); err != nil {
		t.Fatal(err)
	}
	if record("cfg/a.txt").UpdatedAt != aBefore.UpdatedAt {
		t.Errorf("unchanged file re-uploaded")
	}
	if record("cfg/sub/b.txt").SHA256 == bBefore.SHA256 {
		t.Errorf("changed file not uploaded")
	}

	// --delete removes remote files missing locally.
	os.Remove(filepath.Join(src, "a.txt"))
	if err := syncUp(ctx, client, src, "sync-test", "cfg/", true, false); err != nil {
		t.Fatal(err)
	}
	if keys := remoteKeys(t, client); len(keys) != 1 || keys[0] != "cfg/sub/b.txt" {
		t.Fatalf("after delete sync %v", keys)
	}

	// Down into a fresh directory, keeping modification times.
	writeFile(t, filepath.Join(src, "a.txt"), "alpha")
	os.Chtimes(filepath.Join(src, "a.txt"), old, old)
	syncUp(ctx, client, src, "sync-test", "cfg/", false, false)

	dst := filepath.Join(t.TempDir(), "out")
	if err := syncDown(ctx, client, "sync-test", "cfg/", dst, false, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dst, "sub", "b.txt"))
	if string(data) != "beta v2" {
		t.Errorf("downloaded %q", data)
	}
	if info, err := os.Stat(filepath.Join(dst, "a.txt")); err != nil || !info.ModTime().Equal(old) {
		t.Errorf("mtime not kept: %v %v", info.ModTime(), err)
	}

	// Down with --delete removes local extras, leaving matching files alone.
	writeFile(t, filepath.Join(dst, "extra.txt"), "x")
	stat, _ := os.Stat(filepath.Join(dst, "sub", "b.txt"))
	if err := syncDown(ctx, client, "sync-test", "cfg/", dst, true, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "extra.txt")); !os.IsNotExist(err) {
		t.Error("local extra kept")
	}
	if again, _ := os.Stat(filepath.Join(dst, "sub", "b.txt")); !again.ModTime().Equal(stat.ModTime()) || again.Size() != stat.Size() {
		t.Error("matching file rewritten")
	}
}

// Syncing a symlinked directory uploads its files, and --delete must never
// mistake it for an empty directory.
func TestSyncUpThroughSymlink(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	real := t.TempDir()
	writeFile(t, filepath.Join(real, "keep.txt"), "keep")
	if err := syncUp(ctx, client, real, "sync-test", "ln/", false, false); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	writeFile(t, filepath.Join(real, "new.txt"), "new")
	if err := syncUp(ctx, client, link, "sync-test", "ln/", true, false); err != nil {
		t.Fatal(err)
	}
	if keys := remoteKeys(t, client); len(keys) != 2 || keys[0] != "ln/keep.txt" || keys[1] != "ln/new.txt" {
		t.Fatalf("after syncing through a symlink with --delete: %v", keys)
	}
}

// Downloads get normal file permissions, not the temporary file's 0600.
func TestDownloadMode(t *testing.T) {
	client, _ := newFilesServer(t)
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "f.txt")
	writeFile(t, src, "x")
	if err := uploadFile(ctx, client, src, "sync-test", "mode/f.txt"); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(t.TempDir(), "f.txt")
	if err := downloadFile(ctx, client, "sync-test", "mode/f.txt", dst); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dst); info.Mode().Perm() != 0644 {
		t.Errorf("new download mode %o", info.Mode().Perm())
	}
	os.Chmod(dst, 0600)
	downloadFile(ctx, client, "sync-test", "mode/f.txt", dst)
	if info, _ := os.Stat(dst); info.Mode().Perm() != 0600 {
		t.Errorf("replaced file lost its mode: %o", info.Mode().Perm())
	}
}
