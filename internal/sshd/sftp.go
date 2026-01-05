package sshd

import (
	"io"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/internal/log"
	"github.com/pkg/sftp"
)

// SftpHandler handler for SFTP subsystem
func SftpHandler(sess ssh.Session) {
	logger := log.WithGroup("sshd")

	home, err := os.UserHomeDir()
	if err != nil {
		logger.WithError(err).Error("failed to get user home directory")
		return
	}

	serverOptions := []sftp.ServerOption{
		sftp.WithServerWorkingDirectory(home),
	}
	server, err := sftp.NewServer(
		sess,
		serverOptions...,
	)
	if err != nil {
		logger.WithError(err).Error("sftp server init error")
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
	} else if err != nil {
		logger.WithError(err).Error("SFTP server completed with error")
	}
}
