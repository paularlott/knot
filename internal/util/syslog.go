//go:build !windows

package util

import (
	"log/syslog"
	"os"
	"os/exec"

	"github.com/paularlott/knot/internal/log"
)

func RedirectToSyslog(cmd *exec.Cmd) {
	// Redirect output to syslog
	sysLogger, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "code-server")
	if err != nil {
		log.WithError(err).Error("code-server: error creating syslog writer:")

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return
	}

	cmd.Stdout = sysLogger
	cmd.Stderr = sysLogger
}
