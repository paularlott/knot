//go:build integration

package suites

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paularlott/knot/integration-tests/harness"
)

// TestNFSWithTools uses a bucket mounted over NFS as people do, with tar,
// rsync and git, which lean on what a plain write does not: exclusive
// creates, renames over files, lock files, and setting times and modes. A
// tree of files of every kind goes in each way and comes back the same:
// content, modes, links, empty folders and modification times.
func TestNFSWithTools(t *testing.T) {
	harness.Feature(t, "file-storage")
	skipUnlessNFS(t)
	for _, tool := range []string{"tar", "rsync", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("needs %s", tool)
		}
	}
	root := filesRoot(t)
	s, admin := bootDedicated(t, "nfstools", "--files-path", filepath.Join(root, "store-tools"))
	ctx, cancel := testCtx(600)
	defer cancel()
	if _, err := admin.Client.CreateFileBucket(ctx, "tools"); err != nil {
		t.Fatal(err)
	}
	mnt := mountNFS(t, s, admin, filepath.Join(root, "tools-mnt"))
	bucket := filepath.Join(mnt, "tools")
	// Local copies on the machine's own disk, which keeps every kind of
	// entry; KNOT_TEST_FILES_ROOT may be a disk that doesn't.
	local := t.TempDir()
	src := makeToolTree(t, filepath.Join(local, "src"))

	run := func(t *testing.T, dir string, name string, args ...string) string {
		t.Helper()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "HOME="+local, "GIT_CONFIG_NOSYSTEM=1",
			"GIT_AUTHOR_NAME=it", "GIT_AUTHOR_EMAIL=it@example.com", "GIT_COMMITTER_NAME=it", "GIT_COMMITTER_EMAIL=it@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	t.Run("tar", func(t *testing.T) {
		archive := filepath.Join(local, "tools.tar")
		run(t, src, "tar", "-cf", archive, ".")
		dst := filepath.Join(bucket, "tar")
		os.MkdirAll(dst, 0755)
		run(t, dst, "tar", "-xpf", archive)
		sameTree(t, src, dst, true)
		// And out again, from the mount.
		back := filepath.Join(local, "tools-tar-back")
		os.MkdirAll(back, 0755)
		run(t, dst, "tar", "-cf", archive+".2", ".")
		run(t, back, "tar", "-xpf", archive+".2")
		sameTree(t, src, back, true)
	})

	t.Run("rsync", func(t *testing.T) {
		dst := filepath.Join(bucket, "rsync")
		run(t, local, "rsync", "-a", src+"/", dst+"/")
		sameTree(t, src, dst, true)
		// Nothing left to do, by checksum, modes or times.
		if out := changes(run(t, local, "rsync", "-a", "--checksum", "--dry-run", "--itemize-changes", src+"/", dst+"/")); out != "" {
			t.Errorf("rsync finds differences after a copy:\n%s", out)
		}
		// One file changed: just it is sent, written to a temporary file and
		// renamed over the old one.
		os.WriteFile(filepath.Join(src, "d01", "f003.txt"), []byte("changed by rsync\n"), 0644)
		future := time.Now().Add(time.Hour).Truncate(time.Second)
		os.Chtimes(filepath.Join(src, "d01", "f003.txt"), future, future)
		out := changes(run(t, local, "rsync", "-a", "--itemize-changes", src+"/", dst+"/"))
		if !strings.Contains(out, "d01/f003.txt") || strings.Count(out, "\n") > 1 {
			t.Errorf("rsync of one changed file sent:\n%s", out)
		}
		sameTree(t, src, dst, true)
		// --delete removes what the source no longer has.
		os.Remove(filepath.Join(src, "d02", "f004.txt"))
		run(t, local, "rsync", "-a", "--delete", src+"/", dst+"/")
		if _, err := os.Lstat(filepath.Join(dst, "d02", "f004.txt")); !os.IsNotExist(err) {
			t.Errorf("rsync --delete left the file: %v", err)
		}
		// And back out of the mount.
		back := filepath.Join(local, "tools-rsync-back")
		run(t, local, "rsync", "-a", dst+"/", back+"/")
		sameTree(t, src, back, true)
	})

	t.Run("git", func(t *testing.T) {
		// A working repository on the mount: commits, a repack, a check.
		repo := filepath.Join(bucket, "repo")
		os.MkdirAll(repo, 0755)
		run(t, local, "rsync", "-a", src+"/", repo+"/")
		run(t, repo, "git", "init", "-q", "-b", "main")
		run(t, repo, "git", "add", "-A")
		run(t, repo, "git", "commit", "-q", "-m", "first")
		os.WriteFile(filepath.Join(repo, "d00", "f000.txt"), []byte("second\n"), 0644)
		run(t, repo, "git", "commit", "-q", "-am", "second")
		run(t, repo, "git", "gc", "-q")
		run(t, repo, "git", "fsck", "--strict")
		if out := run(t, repo, "git", "status", "--porcelain"); strings.TrimSpace(out) != "" {
			t.Errorf("git status after commits:\n%s", out)
		}

		// A bare repository on the mount, cloned and pushed to.
		bare := filepath.Join(bucket, "bare.git")
		run(t, local, "git", "clone", "-q", "--bare", repo, bare)
		clone := filepath.Join(local, "tools-clone")
		run(t, local, "git", "clone", "-q", bare, clone)
		os.WriteFile(filepath.Join(clone, "pushed.txt"), []byte("pushed\n"), 0644)
		run(t, clone, "git", "add", "pushed.txt")
		run(t, clone, "git", "commit", "-q", "-m", "third")
		run(t, clone, "git", "push", "-q", "origin", "main")
		run(t, bare, "git", "fsck", "--strict")
		if out := run(t, bare, "git", "log", "--format=%s", "main"); strings.Fields(out)[0] != "third" {
			t.Errorf("the push did not reach the bare repository:\n%s", out)
		}
		again := filepath.Join(local, "tools-clone-2")
		run(t, local, "git", "clone", "-q", bare, again)
		if b, err := os.ReadFile(filepath.Join(again, "pushed.txt")); err != nil || string(b) != "pushed\n" {
			t.Errorf("a fresh clone lacks the pushed file: %q %v", b, err)
		}
	})
}

// changes is rsync's --itemize-changes output without the lines for a
// link's permission bits alone (.L...p): macOS gives a link a mode of its
// own, which rsync there sets every time, where the NFS server, as Linux
// and S3 do, shows every link as 0777 and leaves a chmod of one alone.
func changes(out string) string {
	var keep []string
	for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
		if l != "" && !strings.HasPrefix(l, ".L...p..... ") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// makeToolTree makes a tree of every kind of entry at dir: text files in
// folders, scripts, a private file, a large file, links to a file and a
// folder, a dangling link, empty folders, and times in the past.
func makeToolTree(t *testing.T, dir string) string {
	t.Helper()
	os.RemoveAll(dir)
	for d := 0; d < 5; d++ {
		for f := 0; f < 20; f++ {
			p := filepath.Join(dir, fmt.Sprintf("d%02d", d), fmt.Sprintf("f%03d.txt", f))
			os.MkdirAll(filepath.Dir(p), 0755)
			if err := os.WriteFile(p, []byte(fmt.Sprintf("file %d/%d\n", d, f)), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	os.MkdirAll(filepath.Join(dir, "bin"), 0755)
	os.WriteFile(filepath.Join(dir, "bin", "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0755)
	os.WriteFile(filepath.Join(dir, "private"), []byte("secret\n"), 0600)
	big := make([]byte, 5<<20+123)
	rand.Read(big)
	os.WriteFile(filepath.Join(dir, "big.bin"), big, 0644)
	os.Symlink("bin/run.sh", filepath.Join(dir, "run"))
	os.Symlink("d01", filepath.Join(dir, "linkdir"))
	os.Symlink("nowhere", filepath.Join(dir, "dangling"))
	os.MkdirAll(filepath.Join(dir, "empty", "deeper"), 0755)
	os.MkdirAll(filepath.Join(dir, "locked"), 0700)
	past := time.Date(2024, 3, 4, 5, 6, 7, 0, time.UTC)
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.Type()&fs.ModeSymlink == 0 {
			os.Chtimes(p, past, past)
		}
		return nil
	})
	// Folders last, as writing into them changed their times.
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			os.Chtimes(p, past, past)
		}
		return nil
	})
	return dir
}

// sameTree checks that dst holds what src does: the same entries, kinds,
// permission bits, link targets and content, and, with times, the same
// modification times of files to the second.
func sameTree(t *testing.T, src, dst string, times bool) {
	t.Helper()
	seen := map[string]bool{}
	problems := 0
	report := func(format string, args ...any) {
		problems++
		if problems <= 20 {
			t.Errorf(format, args...)
		}
	}
	filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		seen[rel] = true
		if rel == "." {
			return nil
		}
		a, _ := os.Lstat(p)
		b, err := os.Lstat(filepath.Join(dst, rel))
		if err != nil {
			report("%s: missing: %v", rel, err)
			return nil
		}
		if a.Mode().Type() != b.Mode().Type() {
			report("%s: is %v, want %v", rel, b.Mode().Type(), a.Mode().Type())
			return nil
		}
		switch {
		case a.Mode()&fs.ModeSymlink != 0:
			ta, _ := os.Readlink(p)
			tb, _ := os.Readlink(filepath.Join(dst, rel))
			if ta != tb {
				report("%s: links to %q, want %q", rel, tb, ta)
			}
			return nil
		case a.Mode().Perm() != b.Mode().Perm():
			report("%s: mode %04o, want %04o", rel, b.Mode().Perm(), a.Mode().Perm())
		}
		if a.Mode().IsRegular() {
			ca, _ := os.ReadFile(p)
			cb, _ := os.ReadFile(filepath.Join(dst, rel))
			if !bytes.Equal(ca, cb) {
				report("%s: content differs (%d bytes, want %d)", rel, len(cb), len(ca))
			}
			if times && !a.ModTime().Truncate(time.Second).Equal(b.ModTime().Truncate(time.Second)) {
				report("%s: modified %s, want %s", rel, b.ModTime().UTC(), a.ModTime().UTC())
			}
		}
		return nil
	})
	filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(dst, p)
		if !seen[rel] && !strings.HasPrefix(filepath.Base(rel), "._") {
			report("%s: not in the source", rel)
		}
		return nil
	})
	if problems > 20 {
		t.Errorf("and %d more differences", problems-20)
	}
}
