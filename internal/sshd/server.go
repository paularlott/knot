package sshd

import (
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/util"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
	"github.com/paularlott/knot/internal/log"
	gossh "golang.org/x/crypto/ssh"
)

var (
	preferredShell string = "bash"
)

func ListenAndServe(port int, privateKeyPEM string) {
	log.Info("sshd: starting on port", "port", port)

	// Generate a new private key if one is not provided
	if privateKeyPEM == "" {
		log.Info("sshd: generating new private key")

		var err error
		privateKeyPEM, err = GenerateEd25519PrivateKey()
		if err != nil {
			log.Fatal("sshd: failed to generate private key:", "err", err)
		}
	}

	signer, err := gossh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		log.Fatal("sshd: failed to parse SSH private key:", "err", err)
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
			log.Debug("sshd: local port forwarding requested", "dhost", dhost, "dport", dport)
			return true
		}),
	}

	go func() {
		log.Fatal("ssh:", "ssh", ssh_server.ListenAndServe())
	}()
}

func defaultHandler(s ssh.Session) {
	ptyReq, winCh, isPty := s.Pty()
	if isPty {

		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal("failed to get user home directory:", "err", err)
		}

		// Check requested shell exists, if not find one
		selectedShell := util.CheckShells(preferredShell)
		if selectedShell == "" {
			log.Error("sshd: no valid shell found")
			s.Exit(1)
			return
		}

		log.Debug("sshd: starting shell", "selectedShell", selectedShell)

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
				log.WithError(err).Error("sshd: Failed to open listener for agent forwarding")
			}
			defer l.Close()
			go ssh.ForwardAgentConnections(l, s)

			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
		}

		if tty, err = pty.Start(cmd); err != nil {
			log.WithError(err).Error("sshd: Failed to start PTY")
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
			log.Error("sshd: no command provided")
			io.WriteString(s, "No command provided.\n")
			s.Exit(1)
		}
	}
}

func SetShell(shell string) {
	preferredShell = shell
}
