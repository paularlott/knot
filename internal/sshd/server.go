package sshd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

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

// prepareCommand sets up a command with proper home directory, shell, and environment
func prepareCommand(s ssh.Session, command string, extraEnv ...string) (*exec.Cmd, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "", err
	}

	selectedShell := util.CheckShells(preferredShell)
	if selectedShell == "" {
		return nil, "", fmt.Errorf("no valid shell found")
	}

	var cmd *exec.Cmd
	if command != "" {
		// Execute command through shell for variable expansion
		cmd = exec.Command(selectedShell, "-c", command)
	} else {
		// No command - start interactive shell
		cmd = exec.Command(selectedShell, "-l")
	}

	cmd.Dir = home
	cmd.Env = append(os.Environ(), s.Environ()...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("HOME=%s", home))
	cmd.Env = append(cmd.Env, fmt.Sprintf("USER=%s", s.User()))
	cmd.Env = append(cmd.Env, extraEnv...)

	return cmd, home, nil
}

func ListenAndServe(port int, privateKeyPEM string) {
	logger := log.WithGroup("sshd")
	logger.Info("starting on port", "port", port)

	// Generate a new private key if one is not provided
	if privateKeyPEM == "" {
		logger.Info("generating new private key")

		var err error
		privateKeyPEM, err = GenerateEd25519PrivateKey()
		if err != nil {
			logger.Fatal("failed to generate private key:", "err", err)
		}
	}

	signer, err := gossh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		logger.Fatal("failed to parse SSH private key:", "err", err)
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
			return true
		}),
	}

	go func() {
		logger.Fatal("ssh server error", "err", ssh_server.ListenAndServe())
	}()
}

// executeNonPtyCommand executes a command in non-PTY mode with proper I/O handling
func executeNonPtyCommand(s ssh.Session, command string) {
	logger := log.WithGroup("sshd")

	cmd, _, err := prepareCommand(s, command)
	if err != nil {
		logger.WithError(err).Error("failed to prepare command")
		s.Exit(1)
		return
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.WithError(err).Error("failed to create stdout pipe")
		s.Exit(1)
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		logger.WithError(err).Error("failed to create stderr pipe")
		s.Exit(1)
		return
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		logger.WithError(err).Error("failed to create stdin pipe")
		s.Exit(1)
		return
	}

	if err := cmd.Start(); err != nil {
		logger.WithError(err).Error("failed to start command")
		s.Exit(1)
		return
	}

	// Channel to track output completion
	done := make(chan struct{}, 2)

	// Copy stdin from session to command (don't wait for this to avoid deadlock)
	go func() {
		io.Copy(stdin, s)
		stdin.Close()
	}()

	// Copy stdout from command to session
	go func() {
		io.Copy(s, stdout)
		done <- struct{}{}
	}()

	// Copy stderr from command to session stderr
	go func() {
		io.Copy(s.Stderr(), stderr)
		done <- struct{}{}
	}()

	// Wait for output streams to finish
	<-done
	<-done

	// Wait for command to finish
	err = cmd.Wait()

	// Close output streams (stdin will be closed by its goroutine)
	stdout.Close()
	stderr.Close()

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			s.Exit(exitErr.ExitCode())
		} else {
			s.Exit(1)
		}
	} else {
		s.Exit(0)
	}
}

// runPtySession runs a command on a pty for an ssh session until the command exits or the
// client goes away. A command left behind by a client that disconnects is hung up, as
// OpenSSH does, so a shell gets to save its history rather than lingering.
func runPtySession(s ssh.Session, cmd *exec.Cmd, tty *os.File, winCh <-chan ssh.Window) {
	// The pty has to stay open until the command has exited, see util.HangUpShell
	defer tty.Close()

	go func() {
		for win := range winCh {
			setWinsize(tty, win.Width, win.Height)
		}
	}()
	go io.Copy(tty, s) // stdin

	waitErr := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		waitErr <- cmd.Wait()
		close(exited)
	}()

	outputDone := make(chan struct{})
	go func() {
		io.Copy(s, tty) // stdout
		close(outputDone)
	}()

	select {
	case <-exited:
		// The command ended on its own, give the last of its output a moment to get out
		select {
		case <-outputDone:
		case <-s.Context().Done():
		case <-time.After(time.Second):
		}
	case <-outputDone:
		// The pty closed, or output can't reach the client any more. A shell that is exiting
		// normally is closing the pty; leave it alone so it can finish saving its history.
		select {
		case <-exited:
		case <-time.After(time.Second):
			util.HangUpShell(cmd.Process, tty, exited)
		}
	case <-s.Context().Done():
		// The client has gone away, possibly while the shell was idle
		util.HangUpShell(cmd.Process, tty, exited)
	}

	<-exited
	if err := <-waitErr; err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			s.Exit(exitErr.ExitCode())
		} else {
			s.Exit(1)
		}
	} else {
		s.Exit(0)
	}
}

func defaultHandler(s ssh.Session) {
	logger := log.WithGroup("sshd")
	ptyReq, winCh, isPty := s.Pty()
	if isPty {
		// Check if a command was specified with PTY
		commands := s.Command()
		if len(commands) > 0 {
			// Execute command in PTY mode
			cmd, _, err := prepareCommand(s, s.RawCommand(), fmt.Sprintf("TERM=%s", ptyReq.Term))
			if err != nil {
				logger.WithError(err).Error("failed to prepare command")
				s.Exit(1)
				return
			}

			var tty *os.File
			if tty, err = pty.Start(cmd); err != nil {
				logger.WithError(err).Error("Failed to start PTY command")
				s.Exit(1)
				return
			}

			runPtySession(s, cmd, tty, winCh)
			return
		}

		// No command specified, start interactive shell

		cmd, _, err := prepareCommand(s, "", fmt.Sprintf("TERM=%s", ptyReq.Term))
		if err != nil {
			logger.WithError(err).Error("failed to prepare shell")
			s.Exit(1)
			return
		}

		var tty *os.File

		// If agent forwarding then start the agent listener and add the env var
		if ssh.AgentRequested(s) {
			l, err := ssh.NewAgentListener()
			if err != nil {
				logger.WithError(err).Error("Failed to open listener for agent forwarding")
			}
			defer l.Close()
			go ssh.ForwardAgentConnections(l, s)

			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
		}

		if tty, err = pty.Start(cmd); err != nil {
			logger.WithError(err).Error("Failed to start PTY")
			s.Exit(1)
			return
		}

		runPtySession(s, cmd, tty, winCh)
	} else {
		// Non-PTY mode: execute command or start shell
		commands := s.Command()
		if len(commands) > 0 {
			// Execute the provided command
			executeNonPtyCommand(s, s.RawCommand())
		} else {
			// No command - start shell for clients like VSCode
			executeNonPtyCommand(s, "")
		}
	}
}

func SetShell(shell string) {
	preferredShell = shell
}
