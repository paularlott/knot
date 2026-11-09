//go:build integration && windows

package suites

import (
	"errors"
	"os"
)

// The tests that lock files mount NFS, which they do on macOS only.

func lockFile(*os.File, bool, bool) error { return errors.New("no flock on Windows") }

func unlockFile(*os.File) error { return errors.New("no flock on Windows") }
