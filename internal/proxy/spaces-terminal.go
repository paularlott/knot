package proxy

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/util"
	"github.com/paularlott/knot/util/validate"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func HandleSpacesTerminalProxy(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	spaceId := r.PathValue("space_id")
	if !validate.UUID(spaceId) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	shell := r.PathValue("shell")
	if !validate.Subdomain(shell) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Load the space
	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check user access to the space
	if space.UserId != user.Id && space.SharedWithUserId != user.Id {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Get the agent session, fail if no session or terminal is not enabled
	agentSession := agent_server.GetSession(space.Id)
	if agentSession == nil || (shell == "vscode-terminal" && !agentSession.HasCodeServer) || (shell != "vscode-terminal" && !agentSession.HasTerminal) {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Open a new stream to the agent
	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	if shell == "vscode-tunnel" {
		// Write the terminal command
		err = msg.WriteCommand(stream, msg.CmdVSCodeTunnelTerminal)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	} else {
		// Write the terminal command
		err = msg.WriteCommand(stream, msg.CmdTerminal)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		// Write the terminal request
		err = msg.WriteMessage(stream, &msg.Terminal{
			Shell: shell,
		})
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	// Upgrade the connection to a websocket
	conn := util.UpgradeToWS(w, r)
	if conn == nil {
		log.Error().Msg("error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// stream to websocket
	go func() {
		for {
			buffer := make([]byte, 2048)
			readLength, err := stream.Read(buffer)
			if err != nil {
				log.Error().Msgf("failed to read from terminal: %s", err)
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, buffer[:readLength]); err != nil {
				log.Error().Msgf("failed to send %v bytes to terminal", readLength)
				continue
			}
		}
	}()

	// websocket to stream
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
			data := make([]byte, 2048)
			_, err = r.Read(data)
			if err != nil {
				log.Error().Msgf("failed to read data type from websocket: %s", err)
				return
			}

			data = bytes.Trim(data, "\x00")

			if mt == websocket.BinaryMessage && len(data) > 0 {
				if data[0] == 1 {
					ttySize := &msg.TerminalWindowSize{}
					resizeMessage := bytes.Trim(data[1:], " \n\r\t\x00\x01")
					if err := json.Unmarshal(resizeMessage, ttySize); err != nil {
						log.Error().Msgf("failed to unmarshal resize message '%s': %s", string(resizeMessage), err)
						continue
					}
					log.Debug().Msgf("resizing tty to use %v x %v", ttySize.Rows, ttySize.Cols)

					if err := msg.WriteCommand(stream, msg.MSG_TERMINAL_RESIZE); err != nil {
						log.Error().Msgf("error writing command to stream: %s", err)
						return
					}

					if err := msg.WriteMessage(stream, ttySize); err != nil {
						log.Error().Msgf("error writing message to stream: %s", err)
						return
					}
				}
			}
		} else if mt == websocket.TextMessage {
			buffer := make([]byte, 2048)
			for {
				n, err := r.Read(buffer)
				if err != nil && err != io.EOF {
					log.Error().Msgf("error reading from websocket: %s", err)
					return
				}

				if n > 0 {
					if err := msg.WriteCommand(stream, msg.MSG_TERMINAL_DATA); err != nil {
						log.Error().Msgf("error writing command to stream: %s", err)
						return
					}

					// Write the size of the payload using binary.BigEndian
					payloadSize := uint32(n)
					sizeBytes := make([]byte, 4)
					binary.BigEndian.PutUint32(sizeBytes, payloadSize)
					if _, err := stream.Write(sizeBytes); err != nil {
						log.Error().Msgf("error writing size to stream: %s", err)
						return
					}

					// Write the buffer to the stream
					if _, err := stream.Write(buffer[:n]); err != nil {
						log.Error().Msgf("error writing buffer to stream: %s", err)
						return
					}
				}

				if err == io.EOF {
					break
				}
			}
		}
	}
}
