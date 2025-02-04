package sshd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/util"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
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
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"session":      ssh.DefaultSessionHandler,
			"direct-tcpip": ssh.DirectTCPIPHandler,
		},
		LocalPortForwardingCallback: ssh.LocalPortForwardingCallback(func(ctx ssh.Context, dhost string, dport uint32) bool {
			log.Debug().Msgf("sshd: local port forwarding requested to %s:%d", dhost, dport)
			return true
		}),
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
		selectedShell := util.CheckShells(preferredShell)
		if selectedShell == "" {
			log.Error().Msg("sshd: no valid shell found")
			s.Exit(1)
			return
		}

		log.Debug().Msgf("sshd: starting shell %s", selectedShell)

		var cmd *exec.Cmd
		var tty *os.File

		cmd = exec.Command(selectedShell, "-l")
		cmd.Dir = home
		cmd.Env = append(os.Environ(), s.Environ()...)
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))

		// If agent forwarding then start the agent listener and add the env var
		if ssh.AgentRequested(s) {
			l, err := ssh.NewAgentListener()
			if err != nil {
				log.Error().Err(err).Msg("sshd: Failed to open listener for agent forwarding")
			}
			defer l.Close()
			go ssh.ForwardAgentConnections(l, s)

			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
		}

		if tty, err = pty.Start(cmd); err != nil {
			log.Error().Err(err).Msg("sshd: Failed to start PTY")
			s.Exit(1)
			return
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
		commands := s.Command()
		if len(commands) > 0 {
			cmd := exec.Command(commands[0], commands[1:]...)
			cmd.Env = append(os.Environ(), s.Environ()...)
			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			stdin, _ := cmd.StdinPipe()
			cmd.Start()
			go io.Copy(stdin, s)  // forward input
			go io.Copy(s, stdout) // forward output
			io.Copy(s, stderr)    // forward errors
			cmd.Wait()
		} else {
			log.Error().Msg("sshd: no command provided")
			io.WriteString(s, "No command provided.\n")
			s.Exit(1)
		}
	}
}

func SetShell(shell string) {
	preferredShell = shell
}
