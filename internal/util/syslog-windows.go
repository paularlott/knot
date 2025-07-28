//go:build windows

package util

import (
	"os"
	"os/exec"
)

func RedirectToSyslog(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}
