package sshd

import (
	"fmt"
	"io"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/internal/log"
	"github.com/pkg/sftp"
)

// SftpHandler handler for SFTP subsystem
func SftpHandler(sess ssh.Session) {

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("failed to get user home directory:", "err", err)
	}

	debugStream := io.Discard
	serverOptions := []sftp.ServerOption{
		sftp.WithDebug(debugStream),
		sftp.WithServerWorkingDirectory(home),
	}
	server, err := sftp.NewServer(
		sess,
		serverOptions...,
	)
	if err != nil {
		log.WithError(err).Error("sftp server init error")
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}
}
