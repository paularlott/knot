// Package writedir adds to and removes from folders that are read-only to
// their owner, as a mirror makes them when it copies a read-only folder:
// the folder is opened for its owner while the change is made, then given
// its mode back, as rm -rf and the owner's own tools would.
//
// Only a refusal for permission does this, and only for the folder's owner;
// anything else, or a folder the caller can't change the mode of, fails as
// it would have.
package writedir

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// mu serialises opening and closing folders, so one change never closes a
// folder while another opened it and is still retrying. Only the retry
// after a refusal takes it, so changes that need no opening never wait.
// It is not re-entrant: a fn given to In must not itself call In, Remove,
// Rename, MkdirAll or RemoveAll, or a refusal there would deadlock.
var mu sync.Mutex

// modeBits are the bits a folder is given back: its permissions and the
// setuid, setgid and sticky bits beside them.
const modeBits = fs.ModePerm | fs.ModeSetuid | fs.ModeSetgid | fs.ModeSticky

// opened is a folder opened for a change, and the mode it is given back.
type opened struct {
	dir  string
	mode fs.FileMode
}

// open opens dir for its owner when it is read-only to them, reporting the
// mode to give it back.
func open(dir string) (opened, bool) {
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm()&0700 == 0700 {
		return opened{}, false
	}
	mode := info.Mode() & modeBits
	if os.Chmod(dir, mode|0700) != nil {
		// Some systems refuse a setgid folder's mode from an owner outside
		// its group: open it with its permissions alone.
		if os.Chmod(dir, mode.Perm()|0700) != nil {
			return opened{}, false
		}
	}
	return opened{dir, mode}, true
}

// In runs fn, a change to what the folders dirs hold (creating, renaming
// into or out of, or removing an entry). If fn is refused for permission,
// each of dirs that is read-only to its owner is opened (owner read, write
// and search), fn runs again, and the folders get their modes back. A
// folder that doesn't exist yet stands for the nearest one above it that
// does, which is where a MkdirAll adds.
func In(fn func() error, dirs ...string) error {
	err := fn()
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	mu.Lock()
	defer mu.Unlock()
	var was []opened
	seen := map[string]bool{}
	for _, dir := range dirs {
		dir = existing(dir)
		if dir == "" || seen[dir] {
			continue
		}
		seen[dir] = true
		if o, ok := open(dir); ok {
			was = append(was, o)
		}
	}
	if len(was) == 0 {
		return err
	}
	err = fn()
	restore(was)
	return err
}

// restore gives folders still there back their modes, last opened first,
// or their permissions alone where the special bits are refused.
func restore(was []opened) {
	for i := len(was) - 1; i >= 0; i-- {
		if os.Chmod(was[i].dir, was[i].mode) != nil {
			os.Chmod(was[i].dir, was[i].mode.Perm())
		}
	}
}

// existing returns dir, or the nearest folder above it that exists.
func existing(dir string) string {
	for {
		if _, err := os.Lstat(dir); err == nil {
			return dir
		}
		up := filepath.Dir(dir)
		if up == dir {
			return ""
		}
		dir = up
	}
}

// MkdirAll is os.MkdirAll, adding to a read-only folder above path.
func MkdirAll(path string, perm fs.FileMode) error {
	return In(func() error { return os.MkdirAll(path, perm) }, filepath.Dir(path))
}

// Remove is os.Remove, removing from a read-only folder.
func Remove(path string) error {
	return In(func() error { return os.Remove(path) }, filepath.Dir(path))
}

// Rename is os.Rename, between folders that may be read-only.
func Rename(from, to string) error {
	return In(func() error { return os.Rename(from, to) }, filepath.Dir(from), filepath.Dir(to))
}

// RemoveAll is os.RemoveAll, opening the read-only folders in path's tree as
// it goes, and the one holding path, as rm -rf does. Folders it can't open
// fail the removal as before.
func RemoveAll(path string) error {
	err := In(func() error { return os.RemoveAll(path) }, filepath.Dir(path))
	if !errors.Is(err, fs.ErrPermission) {
		return err
	}
	// Something inside is read-only: open every folder, from the top down,
	// before reading it, then remove the lot. If that still fails, what
	// remains gets its modes back.
	mu.Lock()
	var was []opened
	filepath.WalkDir(path, func(p string, d fs.DirEntry, werr error) error {
		if werr == nil && d.IsDir() {
			if o, ok := open(p); ok {
				was = append(was, o)
			}
		}
		return nil
	})
	mu.Unlock()
	if err = In(func() error { return os.RemoveAll(path) }, filepath.Dir(path)); err != nil {
		mu.Lock()
		restore(was)
		mu.Unlock()
	}
	return err
}
