package api

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/filestore"
	"github.com/paularlott/knot/internal/service"
)

// filesEnv runs the real files handlers behind an HTTP server and returns a
// scriptling environment, as a server-side script gets, acting as alice.
func filesEnv(t *testing.T) (run func(script string) map[string]interface{}, store *filestore.Store) {
	t.Helper()
	prev := config.GetServerConfig()
	config.SetServerConfig(&config.ServerConfig{
		BadgerDB: config.BadgerDBConfig{Enabled: true, Path: t.TempDir()},
		Zone:     "z-files-script",
	})
	t.Cleanup(func() { config.SetServerConfig(prev) })
	model.SetRoleCache(nil)

	store, err := filestore.Open(filestore.Config{Dir: t.TempDir(), Quota: func(id string) (int64, error) { return 8 << 20, nil }})
	if err != nil {
		t.Fatal(err)
	}
	filestore.SetInstance(store)
	t.Cleanup(func() {
		filestore.SetInstance(nil)
		store.Close()
	})

	role := model.NewRole("files-script-"+t.Name(), []uint16{model.PermissionUseFiles, model.PermissionShareBuckets}, "")
	model.SaveRoleToCache(role)
	db := database.GetInstance()
	alice := &model.User{Id: "fs-alice", Username: "scriptalice", Email: "scriptalice@test.local", Active: true, Roles: []string{role.Id}}
	bob := &model.User{Id: "fs-bob", Username: "scriptbob", Email: "scriptbob@test.local", Active: true, Roles: []string{role.Id}}
	for _, u := range []*model.User{alice, bob} {
		if err := db.SaveUser(u, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.SaveGroup(model.NewGroup("scriptdevs", alice.Id, 0, 0, 0, 0)); err != nil {
		t.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/buckets", HandleGetFileBuckets)
	mux.HandleFunc("POST /api/files/buckets", HandleCreateFileBucket)
	mux.HandleFunc("GET /api/files/buckets/{bucket}", HandleGetFileBucket)
	mux.HandleFunc("DELETE /api/files/buckets/{bucket}", HandleDeleteFileBucket)
	mux.HandleFunc("POST /api/files/buckets/{bucket}/share", HandleShareFileBucket)
	mux.HandleFunc("POST /api/files/buckets/{bucket}/unshare", HandleUnshareFileBucket)
	mux.HandleFunc("POST /api/files/buckets/{bucket}/transfer", HandleTransferFileBucket)
	mux.HandleFunc("GET /api/files/usage", HandleGetFileUsage)
	mux.HandleFunc("GET /api/files/list/{bucket}", HandleListFileObjects)
	mux.HandleFunc("GET /api/files/objects/{bucket}/{key...}", HandleGetFileObject)
	mux.HandleFunc("PUT /api/files/objects/{bucket}/{key...}", HandlePutFileObject)
	mux.HandleFunc("DELETE /api/files/objects/{bucket}/{key...}", HandleDeleteFileObject)
	mux.HandleFunc("POST /api/files/copy", HandleCopyFileObject)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), "user", alice)))
	}))
	t.Cleanup(srv.Close)

	client, err := apiclient.NewClient(srv.URL, "token", true)
	if err != nil {
		t.Fatal(err)
	}
	env, _, cleanup, err := service.NewServerScriptlingEnv(client, service.ServerScriptlingOptions{User: alice})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	return func(script string) map[string]interface{} {
		t.Helper()
		if _, err := env.Eval(script); err != nil {
			t.Fatalf("script failed: %v\n%s", err, script)
		}
		v, errObj := env.GetVar("result")
		if errObj != nil {
			t.Fatalf("script set no result: %v", errObj)
		}
		m, ok := v.(map[string]interface{})
		if !ok {
			t.Fatalf("result is %T, want a dict", v)
		}
		return m
	}, store
}

func TestKnotFilesLibrary(t *testing.T) {
	run, store := filesEnv(t)

	// Buckets, text and binary files, folders, copies, and the errors.
	r := run(`
import knot.files as files

result = {}
b = files.create_bucket("scripts")
result["bucket"] = b["name"]
result["display"] = b["display_name"]
files.create_bucket("other")

info = files.write_file("scripts", "app/a.txt", "hello ü")
result["write"] = info
result["text"] = files.read_text("scripts", "app/a.txt")
result["type"] = info["content_type"]

blob = b"\x00\x01\xffabc"
files.write_file("scripts", "bin/x.bin", blob)
result["blob_same"] = files.read_file("scripts", "bin/x.bin") == blob
result["blob_len"] = len(files.read_file("scripts", "bin/x.bin"))
result["blob_type"] = files.list_files("scripts", "bin/")["files"][0]["content_type"]
files.write_file("scripts", "typed.json", "{}", content_type="application/json")
result["typed"] = files.list_files("scripts", "typed.json")["files"][0]["content_type"]

# Keys with spaces, unicode and other characters a URL would rewrite.
for key in ["with space/a b.txt", "uni/é日本.txt", "plus+and&amp=eq?q#h.txt", "dots/a.b..c"]:
    files.write_file("scripts", key, "k:" + key)
result["odd_keys_ok"] = all([files.read_text("scripts", k) == "k:" + k for k in ["with space/a b.txt", "uni/é日本.txt", "plus+and&amp=eq?q#h.txt", "dots/a.b..c"]])

top = files.list_files("scripts")
result["top_files"] = [f["key"] for f in top["files"]]
result["top_folders"] = top["folders"]
result["app_files"] = [f["key"] for f in files.list_files("scripts", "app/")["files"]]
rec = files.list_files("scripts", recursive=True)
result["rec_count"] = len(rec["files"])
result["rec_folders"] = rec["folders"]

result["exists_yes"] = files.file_exists("scripts", "app/a.txt")
result["exists_no"] = files.file_exists("scripts", "app/missing.txt")
result["exists_prefix"] = files.file_exists("scripts", "app")

c = files.copy_file("scripts", "app/a.txt", "other", "copied/a.txt")
result["copy_key"] = c["key"]
result["copy_text"] = files.read_text("other", "copied/a.txt")
files.copy_file("scripts", "app/a.txt", "scripts", "app/b.txt")
result["copy_within"] = files.read_text("scripts", "app/b.txt")

errors = {}
for name, fn in [
    ("read_missing", lambda: files.read_file("scripts", "nope.txt")),
    ("text_missing", lambda: files.read_text("scripts", "nope.txt")),
    ("delete_missing", lambda: files.delete_file("scripts", "nope.txt")),
    ("copy_missing", lambda: files.copy_file("scripts", "nope.txt", "other", "x")),
    ("copy_same", lambda: files.copy_file("scripts", "app/a.txt", "scripts", "app/a.txt")),
    ("no_bucket", lambda: files.list_files("nobucket")),
    ("bad_key", lambda: files.write_file("scripts", "a/../b", "x")),
    ("bad_bucket_name", lambda: files.create_bucket("Bad_Name")),
    ("delete_nonempty", lambda: files.delete_bucket("other")),
    ("share_nobody", lambda: files.share_bucket("scripts")),
    ("share_two", lambda: files.share_bucket("scripts", user="scriptbob", everyone=True)),
]:
    try:
        fn()
        errors[name] = ""
    except Exception as e:
        errors[name] = str(e)
result["errors"] = errors

files.delete_file("scripts", "app/b.txt")
result["deleted"] = not files.file_exists("scripts", "app/b.txt")
result["copy_survives"] = files.read_text("other", "copied/a.txt")
`)
	if r["bucket"] != "scriptalice--scripts" || r["display"] != "scripts" {
		t.Errorf("bucket %v %v", r["bucket"], r["display"])
	}
	w, _ := r["write"].(map[string]interface{})
	// The size must arrive as an integer, not the float64 a bare interface{}
	// JSON decode would give.
	if w["key"] != "app/a.txt" || w["size"] != int64(8) || len(fmt.Sprint(w["sha256"])) != 64 {
		t.Errorf("write result %v (%T)", w, w["size"])
	}
	if r["text"] != "hello ü" || r["type"] != "text/plain; charset=utf-8" {
		t.Errorf("text round trip %q %v", r["text"], r["type"])
	}
	if r["blob_same"] != true || fmt.Sprint(r["blob_len"]) != "6" || r["blob_type"] != "application/octet-stream" || r["typed"] != "application/json" {
		t.Errorf("binary %v %v %v %v", r["blob_same"], r["blob_len"], r["blob_type"], r["typed"])
	}
	if r["odd_keys_ok"] != true {
		t.Error("keys with spaces, unicode or URL characters did not round trip")
	}
	if fmt.Sprint(r["top_files"]) != "[plus+and&amp=eq?q#h.txt typed.json]" || fmt.Sprint(r["top_folders"]) != "[app/ bin/ dots/ uni/ with space/]" {
		t.Errorf("top level: files %v folders %v", r["top_files"], r["top_folders"])
	}
	if fmt.Sprint(r["app_files"]) != "[app/a.txt]" {
		t.Errorf("folder listing %v", r["app_files"])
	}
	if fmt.Sprint(r["rec_count"]) != "7" || fmt.Sprint(r["rec_folders"]) != "[]" {
		t.Errorf("recursive listing %v %v", r["rec_count"], r["rec_folders"])
	}
	if r["exists_yes"] != true || r["exists_no"] != false || r["exists_prefix"] != false {
		t.Errorf("exists %v %v %v", r["exists_yes"], r["exists_no"], r["exists_prefix"])
	}
	if r["copy_key"] != "copied/a.txt" || r["copy_text"] != "hello ü" || r["copy_within"] != "hello ü" || r["copy_survives"] != "hello ü" || r["deleted"] != true {
		t.Errorf("copy: %v %v %v %v %v", r["copy_key"], r["copy_text"], r["copy_within"], r["copy_survives"], r["deleted"])
	}
	errs, _ := r["errors"].(map[string]interface{})
	for name, want := range map[string]string{
		"read_missing": "not found", "text_missing": "not found", "delete_missing": "not found", "copy_missing": "not found", "copy_same": "same file",
		"no_bucket": "not found", "bad_key": "invalid", "bad_bucket_name": "invalid", "delete_nonempty": "not empty", "share_nobody": "exactly one", "share_two": "exactly one",
	} {
		if msg, _ := errs[name].(string); !strings.Contains(strings.ToLower(msg), want) {
			t.Errorf("error for %s = %q, want one containing %q", name, msg, want)
		}
	}

	// Sharing, usage and bucket management.
	r = run(`
import knot.files as files

result = {}
shared = files.share_bucket("scripts", user="scriptbob", access="write")
result["shared"] = shared["shared"]
shared = files.share_bucket("scripts", group="scriptdevs")
result["shared_count"] = len(shared["shared"])
shared = files.share_bucket("scripts", everyone=True)
result["with_all"] = [g["type"] for g in shared["shared"]]
shared = files.unshare_bucket("scripts", everyone=True)
shared = files.unshare_bucket("scripts", group="scriptdevs")
result["after_unshare"] = shared["shared"]

result["usage"] = files.usage()
b = files.get_bucket("scripts")
result["get_count"] = b["count"]
result["get_access"] = b["access"]
result["get_owner"] = b["owner"]
result["list_names"] = [x["name"] for x in files.list_buckets()]

# A bucket with files needs force.
files.delete_bucket("other", force=True)
result["after_delete"] = [x["name"] for x in files.list_buckets()]
try:
    files.get_bucket("other")
    result["gone"] = False
except Exception as e:
    result["gone"] = True
`)
	if sh, _ := r["shared"].([]interface{}); len(sh) != 1 || fmt.Sprint(sh[0]) != "map[access:write name:scriptbob type:user]" {
		t.Errorf("shared %v", r["shared"])
	}
	if types, _ := r["with_all"].([]interface{}); fmt.Sprint(r["shared_count"]) != "2" || len(types) != 3 {
		t.Errorf("sharing %v %v", r["shared_count"], r["with_all"])
	}
	if sh, _ := r["after_unshare"].([]interface{}); len(sh) != 1 {
		t.Errorf("after unshare %v", r["after_unshare"])
	}
	if r["get_access"] != "owner" || r["get_owner"] != "scriptalice" || fmt.Sprint(r["get_count"]) != "7" {
		t.Errorf("get_bucket %v %v %v", r["get_access"], r["get_owner"], r["get_count"])
	}
	u, _ := r["usage"].(map[string]interface{})
	if fmt.Sprint(u["buckets"]) != "2" || fmt.Sprint(u["files"]) != "8" || fmt.Sprint(u["used_bytes"]) == "0" {
		t.Errorf("usage %v", u)
	}
	if fmt.Sprint(r["after_delete"]) != "[scriptalice--scripts]" || r["gone"] != true {
		t.Errorf("after deleting a bucket: %v %v", r["after_delete"], r["gone"])
	}

	// Large and numerous files: a 5 MB round trip, and listing past one page.
	// The many records are merged in as a peer would send them, as 1,200
	// durable writes would take seconds.
	digests, _ := store.Digests("", 0)
	bucket := digests["scriptalice--scripts"].Bucket
	var records []*filestore.Object
	for i := 0; i < 1200; i++ {
		records = append(records, &filestore.Object{
			Bucket: bucket.Name, Key: fmt.Sprintf("many/%05d.txt", i), Generation: bucket.Generation, Size: 1,
			SHA256: strings.Repeat("a", 64), ETag: "x", UpdatedAt: hlc.Now(),
		})
	}
	store.Merge(nil, records)
	r = run(`
import knot.files as files

result = {}
result["many"] = len(files.list_files("scripts", "many/", recursive=True)["files"])
big = b"0123456789abcdef" * (5 * 1024 * 1024 // 16)
files.write_file("scripts", "big.bin", big)
back = files.read_file("scripts", "big.bin")
result["big_len"] = len(back)
result["big_same"] = back == big

# A script holds file content in memory, so one write is capped.
try:
    files.write_file("scripts", "huge.bin", b"x" * (65 * 1024 * 1024))
    result["huge"] = ""
except Exception as e:
    result["huge"] = str(e)
result["huge_exists"] = files.file_exists("scripts", "huge.bin")
`)
	if fmt.Sprint(r["many"]) != "1200" {
		t.Errorf("listing across pages found %v of 1200", r["many"])
	}
	if msg, _ := r["huge"].(string); !strings.Contains(msg, "at most 64 MB") || r["huge_exists"] != false {
		t.Errorf("an over-limit write: %q, stored: %v", r["huge"], r["huge_exists"])
	}
	if fmt.Sprint(r["big_len"]) != fmt.Sprint(5*1024*1024) || r["big_same"] != true {
		t.Errorf("5 MB round trip: %v %v", r["big_len"], r["big_same"])
	}
}
