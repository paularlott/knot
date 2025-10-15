package agent_client

import (
	"encoding/binary"
	"net"
	"os"
	"os/exec"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"

	"github.com/creack/pty"
	"github.com/paularlott/knot/internal/log"
)

func startTerminal(conn net.Conn, shell string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.WithError(err).Error("failed to get home directory:")
		conn.Write([]byte("Failed to get home directory"))
		return
	}

	// Check requested shell exists, if not find one
	selectedShell := util.CheckShells(shell)
	if selectedShell == "" {
		log.Error("no valid shell found")
		conn.Write([]byte("No valid shell found"))
		return
	}

	var tty *os.File
	var cmd *exec.Cmd

	cmd = exec.Command(selectedShell, "-l")
	cmd.Dir = home
	cmd.Env = os.Environ()

	if tty, err = pty.Start(cmd); err != nil {
		log.WithError(err).Error("failed to start shell:")
		conn.Write([]byte("Failed to start shell"))
		return
	}

	// Kill the process and clean up
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			log.Error("unable to kill shell")
		}
		if _, err := cmd.Process.Wait(); err != nil {
			log.Error("unable to wait for shell to exit")
		}
		if err := tty.Close(); err != nil {
			log.Error("unable to close tty")
		}
		if err := conn.Close(); err != nil {
			log.Error("unable to close connection")
		}
	}()

	runTerminal(conn, tty)
}

func startVSCodeTunnelTerminal(conn net.Conn) {

	// Check requested shell exists, if not find one
	var tty *os.File
	var cmd *exec.Cmd
	var err error

	cfg := config.GetAgentConfig()
	cmd = exec.Command("screen", "-d", "-r", cfg.VSCodeTunnel)
	cmd.Env = os.Environ()

	if tty, err = pty.Start(cmd); err != nil {
		log.WithError(err).Error("failed to start shell:")
		return
	}

	// Kill the process and clean up
	defer func() {
		// Send detach control sequence to screen
		if _, err := tty.Write([]byte{0x1b, 0x1b, 0x64}); err != nil {
			log.WithError(err).Error("failed to send detach control sequence to screen:")
		}

		if err := cmd.Process.Kill(); err != nil {
			log.Error("unable to kill shell")
		}
		if _, err := cmd.Process.Wait(); err != nil {
			log.Error("unable to wait for shell to exit")
		}
		if err := tty.Close(); err != nil {
			log.Error("unable to close tty")
		}
		if err := conn.Close(); err != nil {
			log.Error("unable to close connection")
		}
	}()

	runTerminal(conn, tty)
}

func runTerminal(conn net.Conn, tty *os.File) {

	// tty to net
	go func() {
		buffer := make([]byte, 2048)
		for {
			readLength, err := tty.Read(buffer)
			if err != nil {
				log.WithError(err).Error("failed to read from tty:")
				return
			}
			if _, err := conn.Write(buffer[:readLength]); err != nil {
				log.Error("failed to send bytes to terminal", "readLength", readLength)
				continue
			}
		}
	}()

	// net to tty
	for {

		// Read the command bytes from the connection
		cmdTypeBuf := make([]byte, 1)
		_, err := conn.Read(cmdTypeBuf)
		if err != nil {
			log.WithError(err).Error("failed to read command type:")
			return
		}

		if cmdTypeBuf[0] == msg.MSG_TERMINAL_DATA {

			// Read the size of the payload
			sizeBytes := make([]byte, 4)
			if _, err := conn.Read(sizeBytes); err != nil {
				log.WithError(err).Error("failed to read size of payload:")
				return
			}
			payloadSize := binary.BigEndian.Uint32(sizeBytes)

			// Read the payload
			payloadBuf := make([]byte, payloadSize)
			var totalRead uint32 = 0

			for totalRead < payloadSize {
				n, err := conn.Read(payloadBuf[totalRead:])
				if err != nil {
					log.WithError(err).Error("failed to read payload:")
					return
				}
				totalRead += uint32(n)
			}

			if _, err := tty.Write(payloadBuf); err != nil {
				log.Error("failed to write bytes to tty", "payloadSize", payloadSize)
				return
			}

		} else if cmdTypeBuf[0] == msg.MSG_TERMINAL_RESIZE {
			var terminalResize msg.TerminalWindowSize
			if err := msg.ReadMessage(conn, &terminalResize); err != nil {
				log.WithError(err).Error("agent: reading terminal resize message:")
				return
			}

			if err := pty.Setsize(tty, &pty.Winsize{Cols: terminalResize.Cols, Rows: terminalResize.Rows}); err != nil {
				log.WithError(err).Error("failed to resize tty:")
			}
		} else {
			log.Error("unknown command:", "cmdTypeBuf0", cmdTypeBuf[0])
			return
		}
	}
}
