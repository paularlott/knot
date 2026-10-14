//go:build windows

package agent_client

import "os"

// fileUsage has no allocation figures on Windows; callers fall back to the
// file's length.
func fileUsage(info os.FileInfo) (fileID, uint64, bool) {
	return fileID{}, 0, false
}
