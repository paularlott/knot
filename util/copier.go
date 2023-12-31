//
// Copies data between a websocket and stdin / stdout / socket.
//

package util

import (
	"errors"
	"io"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

type copierConnections struct {
  socket net.Conn
  wsConnection *websocket.Conn
}

func NewCopier(socket net.Conn, wsConnection *websocket.Conn) *copierConnections {
  return &copierConnections{
    socket: socket,
    wsConnection: wsConnection,
  }
}

func (connections *copierConnections) Run() error {

  // Copy tcp to websocket
  go func() {
    defer connections.close()

    var n int
    var err error

    buf := make([]byte, 32 * 1024)

    for {
      // Read data from the socket / stdin
      if connections.socket != nil {
        connections.socket.SetReadDeadline(time.Now().Add(10 * time.Second))
        n, err = connections.socket.Read(buf)
      } else {
        n, err = os.Stdin.Read(buf)
      }

      if err != nil && !os.IsTimeout(err) {
        unwrappedErr := errors.Unwrap(err)
        if err != io.EOF && unwrappedErr != nil && unwrappedErr.Error() != "use of closed network connection" {
          log.Error().Msgf("copier: error reading from socket: %s", err.Error())
        }
        return
      }

      // Write data to the websocket
      if err := connections.wsConnection.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
        log.Error().Msgf("copier: error writing to websocket: %s", err.Error())
        return
      }
    }
  }()

  // Copy websocket to tcp
  func() {
    defer connections.close()

    for {
      // Read data from the websocket
      mt, r, err := connections.wsConnection.NextReader()
      if err != nil {
        unwrappedErr := errors.Unwrap(err)
        if unwrappedErr != nil && unwrappedErr.Error() != "use of closed network connection" {
          log.Error().Msgf("copier: error reading from websocket: %s", err.Error())
        }
        return
      }
      if mt != websocket.BinaryMessage {
        log.Error().Msg("copier: received unsupported websocket message type")
        return
      }

      // Write data to the socket / stdout
      if connections.socket != nil {
        _, err = io.Copy(connections.socket, r)
      } else {
        _, err = io.Copy(os.Stdout, r)
      }

      if err != nil {
        log.Error().Msgf("copier: error while writing to socket: %s", err.Error())
        return
      }
    }
  }()

  return nil
}

func (connections *copierConnections) close() {
	connections.wsConnection.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(10 * time.Second))
  connections.wsConnection.Close()

  if connections.socket != nil {
    connections.socket.Close()
  }
}
