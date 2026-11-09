//go:build integration && !windows

package suites

import (
	"os"
	"syscall"
)

// lockFile takes an flock on f, exclusive or shared, waiting for it or not.
func lockFile(f *os.File, exclusive, wait bool) error {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if !wait {
		how |= syscall.LOCK_NB
	}
	return syscall.Flock(int(f.Fd()), how)
}

// unlockFile releases f's flock.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
