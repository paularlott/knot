package command_files

import (
	"strings"
	"syscall"
	"testing"
)

func TestWatchLimitHintInotify(t *testing.T) {
	h := watchLimitHint(syscall.ENOSPC, 20000)
	if !strings.Contains(h, "fs.inotify.max_user_watches") || !strings.Contains(h, "sudo sysctl") {
		t.Errorf("inotify hint: %q", h)
	}
}
