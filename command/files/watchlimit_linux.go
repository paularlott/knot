package command_files

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// watchLimitHint says why folders could not be watched and how to fix it.
// Linux watches each folder with inotify, up to fs.inotify.max_user_watches
// for all of a user's processes, and for every container on a host.
func watchLimitHint(err error, folders int) string {
	switch {
	case errors.Is(err, syscall.ENOSPC):
		limit := "?"
		if data, rerr := os.ReadFile("/proc/sys/fs/inotify/max_user_watches"); rerr == nil {
			limit = strings.TrimSpace(string(data))
		}
		want := folders * 2
		if n, perr := strconv.Atoi(limit); perr == nil && n+folders > want {
			want = n + folders
		}
		return fmt.Sprintf("the limit on watched folders, fs.inotify.max_user_watches = %s, is reached: raise it with sudo sysctl fs.inotify.max_user_watches=%d (on the host, for a container), and add it to /etc/sysctl.conf to keep it", limit, want)
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return "the limit on open files is reached: raise it with ulimit -n"
	case err != nil:
		return err.Error()
	}
	return ""
}
