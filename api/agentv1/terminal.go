package agentv1

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/paularlott/knot/util"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type windowSize struct {
  Rows uint16 `json:"rows"`
  Cols uint16 `json:"cols"`
  X    uint16
  Y    uint16
}

func agentTerminal(w http.ResponseWriter, r *http.Request) {
  shell := chi.URLParam(r, "shell")
  conn := util.UpgradeToWS(w, r)
  if conn == nil {
    log.Error().Msg("error while upgrading to websocket")
    w.WriteHeader(http.StatusInternalServerError)
    return
  }

  // Check requested shell exists, if not find one
  shellPaths := []string{shell, "zsh", "bash", "sh"}
  var tty *os.File
  var cmd *exec.Cmd
  var selectedShell string
  for _, shellPath := range shellPaths {
    var err error

    cmd = exec.Command(shellPath)
    cmd.Env = os.Environ()

    if tty, err = pty.Start(cmd); err == nil {
      selectedShell = shellPath
      break
    }
  }

  if selectedShell == "" {
    log.Error().Msg("no valid shell found")
    conn.WriteMessage(websocket.TextMessage, []byte("No valid shell found"))
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

  // tty to websocket
  go func() {
    for {
      buffer := make([]byte, 1024)
      readLength, err := tty.Read(buffer)
      if err != nil {
        log.Error().Msgf("failed to read from tty: %s", err)
        return
      }
      if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:readLength]); err != nil {
        log.Error().Msgf("failed to send %v bytes to terminal", readLength)
        continue
      }
    }
  }();

  // websocket to tty
	for {
		mt, r, err := conn.NextReader()
		if err != nil {
			unwrappedErr := errors.Unwrap(err)
			if unwrappedErr != nil && unwrappedErr.Error() != "use of closed network connection" {
				log.Error().Msgf("error reading from websocket: %s", err.Error())
			}
			return
		}

    // handle resizing
		if mt == websocket.BinaryMessage {
      data := make([]byte, 1024)
      _, err = r.Read(data)
      if err != nil {
        log.Error().Msgf("failed to read data type from websocket: %s", err)
        return
      }

      data = bytes.Trim(data, "\x00")

      if mt == websocket.BinaryMessage && len(data) > 0 {
        if data[0] == 1 {
          ttySize := &windowSize{}
          resizeMessage := bytes.Trim(data[1:], " \n\r\t\x00\x01")
          if err := json.Unmarshal(resizeMessage, ttySize); err != nil {
            log.Error().Msgf("failed to unmarshal resize message '%s': %s", string(resizeMessage), err)
            continue
          }
          log.Debug().Msgf("resizing tty to use %v x %v", ttySize.Rows, ttySize.Cols)
          if err := pty.Setsize(tty, &pty.Winsize{
            Rows: ttySize.Rows,
            Cols: ttySize.Cols,
          }); err != nil {
            log.Error().Msgf("failed to resize tty, error: %s", err)
          }
        }
      }
		} else if mt == websocket.TextMessage {
			copied, err := io.Copy(tty, r)
			if err != nil {
				log.Error().Msgf("error after copying %d bytes: %s", copied, err)
				return
			}
		}
	}
}
