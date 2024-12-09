//go:build windows

package agentcmd

import (
	"os"
	"os/exec"
)

func redirectToSyslog(cmd *exec.Cmd) {
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
}
