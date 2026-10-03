//go:build !unix

package filestore

import "os"

// lockDir is a no-op where advisory file locks are not available.
func lockDir(dir string) (*os.File, error) { return nil, nil }
