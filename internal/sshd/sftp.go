package sshd

import (
	"fmt"
	"io"
	"os"

	"github.com/gliderlabs/ssh"
	"github.com/pkg/sftp"
	"github.com/rs/zerolog/log"
)

// SftpHandler handler for SFTP subsystem
func SftpHandler(sess ssh.Session) {

	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal().Msgf("failed to get user home directory: %v", err)
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
		log.Printf("sftp server init error: %s\n", err)
		return
	}
	if err := server.Serve(); err == io.EOF {
		server.Close()
		fmt.Println("sftp client exited session.")
	} else if err != nil {
		fmt.Println("sftp server completed with error:", err)
	}
}
