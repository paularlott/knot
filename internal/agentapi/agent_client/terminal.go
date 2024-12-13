package agent_client

import (
	"encoding/binary"
	"net"
	"os"
	"os/exec"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/spf13/viper"

	"github.com/creack/pty"
	"github.com/rs/zerolog/log"
)

func startTerminal(conn net.Conn, shell string) {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Error().Msgf("failed to get home directory: %v", err)
		conn.Write([]byte("Failed to get home directory"))
		return
	}

	// Check requested shell exists, if not find one
	shellPaths := []string{shell, "zsh", "bash", "sh"}
	var tty *os.File
	var cmd *exec.Cmd
	var selectedShell string
	for _, shellPath := range shellPaths {
		var err error

		cmd = exec.Command(shellPath, "-l")
		cmd.Dir = home
		cmd.Env = os.Environ()

		if tty, err = pty.Start(cmd); err == nil {
			selectedShell = shellPath
			break
		}
	}

	if selectedShell == "" {
		log.Error().Msg("no valid shell found")
		conn.Write([]byte("No valid shell found"))
		return
	}

	// Kill the process and clean up
	defer func() {
		if err := cmd.Process.Kill(); err != nil {
			log.Error().Msgf("unable to kill shell")
		}
		if _, err := cmd.Process.Wait(); err != nil {
			log.Error().Msgf("unable to wait for shell to exit")
		}
		if err := tty.Close(); err != nil {
			log.Error().Msgf("unable to close tty")
		}
		if err := conn.Close(); err != nil {
			log.Error().Msgf("unable to close connection")
		}
	}()

	runTerminal(conn, tty)
}

func startVSCodeTunnelTerminal(conn net.Conn) {

	// Check requested shell exists, if not find one
	var tty *os.File
	var cmd *exec.Cmd
	var err error

	cmd = exec.Command("screen", "-d", "-r", viper.GetString("agent.vscode_tunnel"))
	cmd.Env = os.Environ()

	if tty, err = pty.Start(cmd); err != nil {
		log.Error().Msgf("failed to start shell: %s", err)
		return
	}

	// Kill the process and clean up
	defer func() {
		// Send detach control sequence to screen
		if _, err := tty.Write([]byte{0x1b, 0x1b, 0x64}); err != nil {
			log.Error().Msgf("failed to send detach control sequence to screen: %s", err)
		}

		if err := cmd.Process.Kill(); err != nil {
			log.Error().Msgf("unable to kill shell")
		}
		if _, err := cmd.Process.Wait(); err != nil {
			log.Error().Msgf("unable to wait for shell to exit")
		}
		if err := tty.Close(); err != nil {
			log.Error().Msgf("unable to close tty")
		}
		if err := conn.Close(); err != nil {
			log.Error().Msgf("unable to close connection")
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
				log.Error().Msgf("failed to read from tty: %s", err)
				return
			}
			if _, err := conn.Write(buffer[:readLength]); err != nil {
				log.Error().Msgf("failed to send %v bytes to terminal", readLength)
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
			log.Error().Msgf("failed to read command type: %v", err)
			return
		}

		if cmdTypeBuf[0] == msg.MSG_TERMINAL_DATA {

			// Read the size of the payload
			sizeBytes := make([]byte, 4)
			if _, err := conn.Read(sizeBytes); err != nil {
				log.Error().Msgf("failed to read size of payload: %v", err)
				return
			}
			payloadSize := binary.BigEndian.Uint32(sizeBytes)

			// Read the payload
			payloadBuf := make([]byte, payloadSize)
			var totalRead uint32 = 0

			for totalRead < payloadSize {
				n, err := conn.Read(payloadBuf[totalRead:])
				if err != nil {
					log.Error().Msgf("failed to read payload: %v", err)
					return
				}
				totalRead += uint32(n)
			}

			if _, err := tty.Write(payloadBuf); err != nil {
				log.Error().Msgf("failed to write %v bytes to tty", payloadSize)
				return
			}

		} else if cmdTypeBuf[0] == msg.MSG_TERMINAL_RESIZE {
			var terminalResize msg.TerminalWindowSize
			if err := msg.ReadMessage(conn, &terminalResize); err != nil {
				log.Error().Msgf("agent: reading terminal resize message: %v", err)
				return
			}

			if err := pty.Setsize(tty, &pty.Winsize{Cols: terminalResize.Cols, Rows: terminalResize.Rows}); err != nil {
				log.Error().Msgf("failed to resize tty: %s", err)
			}
		} else {
			log.Error().Msgf("unknown command: %d", cmdTypeBuf[0])
			return
		}
	}
}
