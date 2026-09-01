//go:build !windows

package spacejobs

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr puts the job's shell in its own process group so signals
// delivered to the knot process don't propagate into running jobs.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
