//go:build unix

package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// lockDir takes an exclusive lock on a storage directory, so two processes,
// such as a server and a restore, never write to it at once.
func lockDir(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, "meta", "lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrStoreInUse
		}
		return nil, err
	}
	return f, nil
}
