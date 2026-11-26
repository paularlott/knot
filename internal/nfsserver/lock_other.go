//go:build !unix

package nfsserver

import "os"

// lockStaging is a no-op where advisory file locks are not available.
func lockStaging(dir string) (*os.File, error) { return nil, nil }
