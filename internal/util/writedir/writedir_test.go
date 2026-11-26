package writedir

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func sealed(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("folder modes don't stop writes on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder modes")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "ro")
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "f.txt"), []byte("f"), 0644)
	os.WriteFile(filepath.Join(dir, "g.txt"), []byte("g"), 0644)
	os.Chmod(filepath.Join(dir, "sub"), 0500)
	os.Chmod(dir, 0555)
	t.Cleanup(func() {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				os.Chmod(p, 0755)
			}
			return nil
		})
	})
	return dir
}

func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info.Mode().Perm()
}

func TestInAddsToReadOnlyFolderAndRestoresMode(t *testing.T) {
	dir := sealed(t)
	p := filepath.Join(dir, "new.txt")
	if err := In(func() error { return os.WriteFile(p, []byte("n"), 0644) }, dir); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); string(b) != "n" || mode(t, dir) != 0555 {
		t.Errorf("content %q, folder mode %o, want n and 0555", b, mode(t, dir))
	}
	// A refusal that isn't for permission is left alone.
	if err := In(func() error { return os.ErrExist }, dir); err != os.ErrExist {
		t.Errorf("err %v", err)
	}
}

func TestMkdirAllRenameRemove(t *testing.T) {
	dir := sealed(t)
	if err := MkdirAll(filepath.Join(dir, "a", "b"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := Rename(filepath.Join(dir, "g.txt"), filepath.Join(dir, "sub", "g.txt")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(filepath.Join(dir, "sub", "f.txt")); err != nil {
		t.Fatal(err)
	}
	if mode(t, dir) != 0555 || mode(t, filepath.Join(dir, "sub")) != 0500 {
		t.Errorf("modes %o %o, want 0555 0500", mode(t, dir), mode(t, filepath.Join(dir, "sub")))
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "g.txt")); err != nil {
		t.Errorf("rename: %v", err)
	}
}

func TestRemoveAllReadOnlyTree(t *testing.T) {
	dir := sealed(t)
	if err := RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("still there: %v", err)
	}
}

// Many writers into one read-only folder at once: each gets in, and the
// folder ends with its mode.
func TestInConcurrent(t *testing.T) {
	dir := sealed(t)
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p := filepath.Join(dir, "c"+string(rune('a'+i%26))+string(rune('0'+i/26)))
			errs <- In(func() error { return os.WriteFile(p, nil, 0644) }, dir)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
	if mode(t, dir) != 0555 {
		t.Errorf("folder mode %o", mode(t, dir))
	}
}

// A read-only folder's setgid and sticky bits come back with its mode.
func TestInKeepsSpecialBits(t *testing.T) {
	dir := sealed(t)
	if err := os.Chmod(dir, 0555|os.ModeSetgid|os.ModeSticky); err != nil {
		t.Skip(err)
	}
	info, _ := os.Stat(dir)
	want := info.Mode() & modeBits // what the file system kept of them
	if err := In(func() error { return os.WriteFile(filepath.Join(dir, "n"), nil, 0644) }, dir); err != nil {
		t.Fatal(err)
	}
	if info, _ := os.Stat(dir); info.Mode()&modeBits != want {
		t.Errorf("mode %v, want %v", info.Mode()&modeBits, want)
	}
}

// A tree holding a folder its owner can't read is removed too.
func TestRemoveAllUnreadableFolder(t *testing.T) {
	dir := sealed(t)
	os.Chmod(filepath.Join(dir, "sub"), 0200)
	if err := RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Errorf("still there: %v", err)
	}
}
