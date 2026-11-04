//go:build integration

package suites

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/integration-tests/harness"
)

// fileWant is how a file in the shared bucket should look whichever way it
// is reached: its permission bits, or a link's target.
type fileWant struct {
	mode os.FileMode
	link string
}

func (w fileWant) String() string {
	if w.link != "" {
		return "link to " + w.link
	}
	return fmt.Sprintf("%04o", w.mode)
}

// TestFileStorageInterchange checks that files are interchangeable across
// two clustered servers, each with its own database and storage: written on
// one with the knot command, the web interface, over NFS and (Knot Pro) S3,
// each reads the same on the other through the knot command, over NFS from
// macOS, over NFS from a Linux space with the bucket mounted, and over S3,
// modes and links included; written from the space, the same holds back on
// the first server; and fsck --repair on one server reaches the other.
func TestFileStorageInterchange(t *testing.T) {
	harness.Feature(t, "file-storage")
	if runtime.GOOS != "darwin" {
		t.Skip("needs macOS, which mounts NFS as a user, with Docker reaching it as host.docker.internal")
	}
	root := filesRoot(t)

	// Two clustered servers, each with its own database and storage.
	key := "0123456789abcdef0123456789abcdef"
	bindA, bindB := freePorts(t, 2)
	nodeA, err := harness.StartServer(cfg, bins, "interchangea", "--files-path", filepath.Join(root, "store-a"),
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindA),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindB))
	if err != nil {
		t.Fatalf("boot server A: %v", err)
	}
	t.Cleanup(nodeA.Stop)
	nodeB, err := harness.StartServer(cfg, bins, "interchangeb", "--files-path", filepath.Join(root, "store-b"),
		"--cluster-key", key,
		"--cluster-bind-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-advertise-addr", fmt.Sprintf("127.0.0.1:%d", bindB),
		"--cluster-peer", fmt.Sprintf("127.0.0.1:%d", bindA))
	if err != nil {
		t.Fatalf("boot server B: %v", err)
	}
	t.Cleanup(nodeB.Stop)
	adminA, err := harness.ProvisionAdmin(nodeA, "admin", "AdminPassw0rd!")
	if err != nil {
		t.Fatal(err)
	}
	var adminB *harness.User
	if !waitForCond(60, func() bool {
		adminB, err = harness.LoginUser(nodeB, "admin", "AdminPassw0rd!")
		return err == nil
	}) {
		t.Fatalf("the admin did not reach server B: %v", err)
	}
	ctx, cancel := testCtx(600)
	defer cancel()
	if _, err := adminA.Client.CreateFileBucket(ctx, "shared"); err != nil {
		t.Fatal(err)
	}
	if !waitForCond(60, func() bool { _, err := adminB.Client.GetFileBucket(ctx, "shared"); return err == nil }) {
		t.Fatal("the bucket did not reach server B")
	}

	run := func(t *testing.T, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "HOME="+root, "RCLONE_CONFIG="+filepath.Join(root, "rclone.conf"))
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s %s: %v\n%s", filepath.Base(name), strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}
	knotOn := func(t *testing.T, s *harness.Server, u *harness.User, args ...string) string {
		t.Helper()
		return run(t, bins.Server, append(args, "--server", s.BaseURL, "--token", u.Token)...)
	}
	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		os.MkdirAll(filepath.Dir(path), 0755)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
		os.Chmod(path, mode)
	}
	want := map[string]fileWant{}

	// --- Written on server A -------------------------------------------

	// The knot command: a copy, and a sync of a tree with a link in it.
	write(filepath.Join(root, "local", "run.sh"), "#!/bin/sh\n", 0755)
	knotOn(t, nodeA, adminA, "file", "cp", filepath.Join(root, "local", "run.sh"), "shared:cli/run.sh")
	want["cli/run.sh"] = fileWant{mode: 0755}
	tree := filepath.Join(root, "tree")
	write(filepath.Join(tree, "data.txt"), "data", 0640)
	write(filepath.Join(tree, "vendor", "pkg", "tool"), "#!/bin/sh\n", 0755)
	os.MkdirAll(filepath.Join(tree, "vendor", "bin"), 0755)
	os.Symlink("../pkg/tool", filepath.Join(tree, "vendor", "bin", "tool"))
	knotOn(t, nodeA, adminA, "file", "sync", tree, "shared:tree")
	want["tree/data.txt"] = fileWant{mode: 0640}
	want["tree/vendor/pkg/tool"] = fileWant{mode: 0755}
	want["tree/vendor/bin/tool"] = fileWant{link: "../pkg/tool"}

	// The web interface: an upload of a new file, which gets the default
	// mode, and the editor saving over a file, which keeps its mode. Both
	// are a PUT with the modification time and no mode, as the browser
	// sends.
	if _, err := adminA.Client.PutFileObject(ctx, "shared", "web/upload.txt", strings.NewReader("uploaded"), 8, "text/plain", apiclient.FileAttrs{Mtime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	want["web/upload.txt"] = fileWant{mode: 0644}
	if _, err := adminA.Client.PutFileObject(ctx, "shared", "cli/run.sh", strings.NewReader("#!/bin/sh\necho edited\n"), 22, "text/plain; charset=utf-8", apiclient.FileAttrs{Mtime: time.Now()}); err != nil {
		t.Fatal(err)
	}

	// Over NFS, mounted from server A.
	mntA := mountNFS(t, nodeA, adminA, filepath.Join(root, "mnt-a"))
	nfsDir := filepath.Join(mntA, "shared", "nfs")
	if err := os.MkdirAll(nfsDir, 0755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(nfsDir, "made.sh"), "#!/bin/sh\necho nfs\n", 0711)
	if err := os.Symlink("made.sh", filepath.Join(nfsDir, "link")); err != nil {
		t.Fatal(err)
	}
	want["nfs/made.sh"] = fileWant{mode: 0711}
	want["nfs/link"] = fileWant{link: "made.sh"}

	// S3 (Knot Pro): rclone with --metadata, and without it, which sends no
	// mode, so a new file gets the default.
	s3 := harness.ProBuild
	if _, err := exec.LookPath("rclone"); err != nil {
		s3 = false
	}
	rcloneOn := func(t *testing.T, s *harness.Server, u *harness.User, args ...string) string {
		t.Helper()
		cmd := exec.Command("rclone", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "RCLONE_CONFIG="+filepath.Join(root, "rclone.conf"),
			"RCLONE_CONFIG_KNOT_TYPE=s3", "RCLONE_CONFIG_KNOT_PROVIDER=Other",
			"RCLONE_CONFIG_KNOT_ENDPOINT="+s.BaseURL+"/s3",
			"RCLONE_CONFIG_KNOT_ACCESS_KEY_ID="+u.Username,
			"RCLONE_CONFIG_KNOT_SECRET_ACCESS_KEY="+u.Token,
			"RCLONE_CONFIG_KNOT_FORCE_PATH_STYLE=true")
		var out bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Run(); err != nil {
			t.Fatalf("rclone %s: %v\n%s", strings.Join(args, " "), err, out.String())
		}
		return out.String()
	}
	os.WriteFile(filepath.Join(root, "rclone.conf"), nil, 0600)
	if s3 {
		write(filepath.Join(root, "s3src", "meta.sh"), "#!/bin/sh\n", 0750)
		rcloneOn(t, nodeA, adminA, "copy", "-M", filepath.Join(root, "s3src"), "knot:admin--shared/s3/")
		write(filepath.Join(root, "s3plain", "plain.txt"), "plain", 0600)
		rcloneOn(t, nodeA, adminA, "copy", filepath.Join(root, "s3plain"), "knot:admin--shared/s3/")
		want["s3/meta.sh"] = fileWant{mode: 0750}
		want["s3/plain.txt"] = fileWant{mode: 0644}
	}

	// A mode in the decimal form knot wrote before 0.42, which fsck --repair
	// on server A rewrites for both servers.
	write(filepath.Join(root, "old", "old.sh"), "#!/bin/sh\n", 0755)
	knotOn(t, nodeA, adminA, "file", "cp", filepath.Join(root, "old", "old.sh"), "shared:fsck/old.sh")
	info, err := adminA.Client.HeadFileObject(ctx, "shared", "fsck/old.sh")
	if err != nil {
		t.Fatal(err)
	}
	info.Meta[apiclient.FileModeMeta] = "33261" // decimal 0100755
	if _, err := adminA.Client.CopyFileObject(ctx, apiclient.FileCopyRequest{
		SourceBucket: "shared", SourceKey: "fsck/old.sh", DestBucket: "shared", DestKey: "fsck/old.sh", Meta: info.Meta,
	}); err != nil {
		t.Fatal(err)
	}
	knotOn(t, nodeA, adminA, "admin", "file", "fsck", "--repair")
	want["fsck/old.sh"] = fileWant{mode: 0755}

	// --- Read on server B ----------------------------------------------

	// Every file reaches server B with its mode or link.
	stored := func(w fileWant) string {
		if w.link != "" {
			return "120777"
		}
		return apiclient.FormatFileMode(w.mode)
	}
	for k, w := range want {
		if !waitForCond(60, func() bool {
			o, err := adminB.Client.HeadFileObject(ctx, "shared", k)
			return err == nil && o.Meta[apiclient.FileModeMeta] == stored(w)
		}) {
			o, err := adminB.Client.HeadFileObject(ctx, "shared", k)
			t.Fatalf("shared:%s on server B: %+v %v, want mode %s", k, o, err, stored(w))
		}
	}

	// check compares a file as one way of reaching it shows it.
	check := func(t *testing.T, how, k string, mode os.FileMode, link string, err error) {
		t.Helper()
		w := want[k]
		switch {
		case err != nil:
			t.Errorf("%s: %s: %v", how, k, err)
		case w.link != "" && link != w.link:
			t.Errorf("%s: %s is %04o / link %q, want %s", how, k, mode, link, w)
		case w.link == "" && (link != "" || mode != w.mode):
			t.Errorf("%s: %s is %04o / link %q, want %s", how, k, mode, link, w)
		}
	}
	statLocal := func(t *testing.T, how, base string) {
		t.Helper()
		for k := range want {
			p := filepath.Join(base, filepath.FromSlash(k))
			info, err := os.Lstat(p)
			var link string
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				link, err = os.Readlink(p)
			}
			var mode os.FileMode
			if info != nil {
				mode = info.Mode().Perm()
			}
			check(t, how, k, mode, link, err)
		}
	}

	t.Run("knot command on B", func(t *testing.T) {
		dl := filepath.Join(root, "sync-from-b")
		knotOn(t, nodeB, adminB, "file", "sync", "shared:", dl)
		statLocal(t, "knot file sync from B", dl)
	})

	t.Run("NFS on B from macOS", func(t *testing.T) {
		mntB := mountNFS(t, nodeB, adminB, filepath.Join(root, "mnt-b"))
		statLocal(t, "NFS on B", filepath.Join(mntB, "shared"))
	})

	if s3 {
		t.Run("S3 on B", func(t *testing.T) {
			var listed []struct {
				Path     string
				Metadata map[string]string
			}
			if err := json.Unmarshal([]byte(rcloneOn(t, nodeB, adminB, "lsjson", "-R", "-M", "--files-only", "knot:admin--shared")), &listed); err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for _, o := range listed {
				if w, ok := want[o.Path]; ok {
					seen[o.Path] = true
					if o.Metadata["mode"] != stored(w) {
						t.Errorf("S3 on B: %s mode %q, want %s", o.Path, o.Metadata["mode"], stored(w))
					}
				}
			}
			if len(seen) != len(want) {
				t.Errorf("S3 on B listed %d of %d files", len(seen), len(want))
			}
			dl := filepath.Join(root, "rclone-from-b")
			rcloneOn(t, nodeB, adminB, "copy", "-M", "knot:admin--shared/cli/", filepath.Join(dl, "cli"))
			rcloneOn(t, nodeB, adminB, "copy", "-M", "knot:admin--shared/s3/", filepath.Join(dl, "s3"))
			for _, k := range []string{"cli/run.sh", "s3/meta.sh", "s3/plain.txt"} {
				info, err := os.Stat(filepath.Join(dl, filepath.FromSlash(k)))
				var mode os.FileMode
				if info != nil {
					mode = info.Mode().Perm()
				}
				check(t, "rclone -M from B", k, mode, "", err)
			}
		})
	}

	t.Run("space on B with the bucket mounted", func(t *testing.T) {
		// The space's user is uid 1000; files show as its own, as the NFS
		// docs advise for containers.
		port := startNFS(t, nodeB, adminB, "--uid", "1000", "--gid", "1000")
		templateId, err := harness.CreateTemplate(nodeB, adminB.Client, uniqueName("it-nfs"), harness.TemplateOptions{
			Volumes: fmt.Sprintf(`volumes:
  ${{ .space.name }}-shared:
    driver: local
    driver_opts:
      type: nfs
      o: addr=host.docker.internal,port=%d,vers=4.0
      device: ":/shared"
`, port),
			Mounts: []string{"${{ .space.name }}-shared:/shared"},
		})
		if err != nil {
			t.Fatal(err)
		}
		spaceId := harness.CreateSpace(t, adminB.Client, uniqueName("it-nfs"), templateId, adminB.Id)
		// Stopped before the NFS server goes, so the mount is released.
		t.Cleanup(func() { harness.DeleteSpaceAndWait(t, adminB.Client, spaceId) })
		harness.WaitForSpaceReady(t, nodeB, adminB.Client, spaceId)

		if out := harness.RunCommand(t, adminB.Client, spaceId, 30, "id", "-u"); strings.TrimSpace(out) != "1000" {
			t.Fatalf("the space user is uid %q, not 1000", strings.TrimSpace(out))
		}

		// Every file shows its mode or link in the space.
		var paths []string
		for k := range want {
			paths = append(paths, "/shared/"+k)
		}
		out := harness.RunCommand(t, adminB.Client, spaceId, 60, "sh", append([]string{"-c",
			`for p in "$@"; do printf '%s|%s|%s|' "$p" "$(stat -c '%a' "$p")" "$(stat -c '%F' "$p")"; readlink "$p" || echo; done`, "sh"}, paths...)...)
		got := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.SplitN(line, "|", 4)
			if len(f) != 4 {
				continue
			}
			k := strings.TrimPrefix(f[0], "/shared/")
			m, _ := strconv.ParseUint(f[1], 8, 32)
			link := ""
			if f[2] == "symbolic link" {
				link = strings.TrimSpace(f[3])
			}
			got[k] = true
			check(t, "space on B", k, os.FileMode(m), link, nil)
		}
		if len(got) != len(want) {
			t.Errorf("the space saw %d of %d files:\n%s", len(got), len(want), out)
		}
		// The link to a file runs, through the link.
		if out := harness.RunCommand(t, adminB.Client, spaceId, 30, "/shared/tree/vendor/bin/tool"); strings.TrimSpace(out) != "" {
			t.Errorf("running through vendor/bin: %q", out)
		}

		// Written in the space: a file, a chmod, a link and a folder reach
		// server A, its knot command and its NFS.
		harness.RunCommand(t, adminB.Client, spaceId, 30, "sh", "-c",
			"mkdir -p /shared/space && printf '#!/bin/sh\\n' > /shared/space/new.sh && chmod 0750 /shared/space/new.sh && "+
				"ln -s new.sh /shared/space/link && chmod 0700 /shared/tree/data.txt")
		back := map[string]fileWant{
			"space/new.sh":  {mode: 0750},
			"space/link":    {link: "new.sh"},
			"tree/data.txt": {mode: 0700},
		}
		for k, w := range back {
			want[k] = w
			if !waitForCond(60, func() bool {
				o, err := adminA.Client.HeadFileObject(ctx, "shared", k)
				return err == nil && o.Meta[apiclient.FileModeMeta] == stored(w)
			}) {
				o, err := adminA.Client.HeadFileObject(ctx, "shared", k)
				t.Errorf("written in the space, shared:%s on server A: %+v %v, want %s", k, o, err, w)
			}
		}
		dl := filepath.Join(root, "sync-from-a")
		knotOn(t, nodeA, adminA, "file", "sync", "shared:", dl)
		for k := range back {
			p := filepath.Join(dl, filepath.FromSlash(k))
			info, err := os.Lstat(p)
			var link string
			var mode os.FileMode
			if err == nil {
				mode = info.Mode().Perm()
				if info.Mode()&os.ModeSymlink != 0 {
					link, err = os.Readlink(p)
				}
			}
			check(t, "knot file sync from A", k, mode, link, err)
		}
		if !waitForCond(30, func() bool {
			info, err := os.Stat(filepath.Join(mntA, "shared", "space", "new.sh"))
			return err == nil && info.Mode().Perm() == 0750
		}) {
			t.Error("written in the space, new.sh did not show 0750 over NFS on A")
		}
	})
}
