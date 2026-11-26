//go:build unix

package nfsserver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockStaging takes an exclusive lock on a staging folder, so two
// nfs-servers never take back, or save, each other's staged copies.
func lockStaging(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, stagingLock), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("staging folder %s is in use by another nfs-server", dir)
		}
		return nil, err
	}
	return f, nil
}
