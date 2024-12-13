package sshd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/build"
	"github.com/rs/zerolog/log"
	gossh "golang.org/x/crypto/ssh"
)

var (
	preferredShell string = "bash"
)

func ListenAndServe(port int, privateKeyPEM string) {
	log.Info().Msgf("sshd: starting on port %d", port)

	// Generate a new private key if one is not provided
	if privateKeyPEM == "" {
		log.Info().Msg("sshd: generating new private key")

		var err error
		privateKeyPEM, err = GenerateEd25519PrivateKey()
		if err != nil {
			log.Fatal().Msgf("sshd: failed to generate private key: %v", err)
		}
	}

	signer, err := gossh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		log.Fatal().Msgf("sshd: failed to parse SSH private key: %v", err)
	}

	ssh_server := ssh.Server{
		Version: "knot " + build.Version,
		//Banner:  "Welcome to knot " + build.Version + "\r\n\r\n",
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": SftpHandler,
		},
		HostSigners:      []ssh.Signer{signer},
		PublicKeyHandler: publicKeyHandler,
		Handler:          defaultHandler,
	}

	go func() {
		log.Fatal().Msgf("ssh: %v", ssh_server.ListenAndServe())
	}()
}

func defaultHandler(s ssh.Session) {
	ptyReq, winCh, isPty := s.Pty()
	if isPty {

		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal().Msgf("failed to get user home directory: %v", err)
		}

		// Check requested shell exists, if not find one
		shellPaths := []string{preferredShell, "zsh", "bash", "sh"}
		var cmd *exec.Cmd
		var tty *os.File
		var selectedShell string
		for _, shellPath := range shellPaths {
			var err error

			cmd = exec.Command(shellPath, "-l")
			cmd.Dir = home
			cmd.Env = os.Environ()
			cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))

			if tty, err = pty.Start(cmd); err == nil {
				selectedShell = shellPath
				break
			}
		}

		if selectedShell == "" {
			log.Fatal().Msg("sshd: no valid shell found")
		}

		go func() {
			for win := range winCh {
				setWinsize(tty, win.Width, win.Height)
			}
		}()
		go func() {
			io.Copy(tty, s) // stdin
		}()
		io.Copy(s, tty) // stdout
		cmd.Wait()
	} else {
		io.WriteString(s, "No PTY requested.\n")
		s.Exit(1)
	}
}

func SetShell(shell string) {
	preferredShell = shell
}
