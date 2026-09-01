//go:build windows

package spacejobs

import (
	"os/exec"
	"syscall"
)

// setSysProcAttr mirrors the Unix Setpgid behaviour: CREATE_NEW_PROCESS_GROUP
// detaches the job's shell from the knot process group so console Ctrl+C
// events don't propagate into running jobs.
func setSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x200} // CREATE_NEW_PROCESS_GROUP
}
