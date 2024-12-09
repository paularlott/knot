//go:build !windows

package agentcmd

import (
	"log/syslog"
	"os"
	"os/exec"

	"github.com/rs/zerolog/log"
)

func redirectToSyslog(cmd *exec.Cmd) {
	// Redirect output to syslog
	sysLogger, err := syslog.New(syslog.LOG_INFO|syslog.LOG_USER, "code-server")
	if err != nil {
		log.Error().Msgf("code-server: error creating syslog writer: %v", err)

		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return
	}

	cmd.Stdout = sysLogger
	cmd.Stderr = sysLogger
}
