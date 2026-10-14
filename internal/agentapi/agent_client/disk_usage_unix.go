//go:build !windows

package agent_client

import (
	"os"
	"syscall"
)

// fileUsage returns a file's identity and the disk space it takes, from the
// blocks allocated to it rather than its length, so sparse files count what
// they use.
func fileUsage(info os.FileInfo) (fileID, uint64, bool) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileID{}, 0, false
	}
	return fileID{dev: uint64(st.Dev), ino: uint64(st.Ino), links: uint64(st.Nlink)}, uint64(st.Blocks) * 512, true
}
