//
// Copies data between a websocket and stdin / stdout / socket.
//

package proxy

import (
	"io"
	"log"
	"net"
	"os"
	"time"

	"github.com/gorilla/websocket"
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
        if err != io.EOF {
          log.Printf("Error reading from socket: %s", err)
        }
        return
      }

      // Write data to the websocket
      if err := connections.wsConnection.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
        log.Printf("Error writing to websocket: %s", err)
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
        log.Printf("Error reading from websocket: %s", err)
        return
      }
      if mt != websocket.BinaryMessage {
        log.Printf("Received unsupported websocket message type")
        return
      }

      // Write data to the socket / stdout
      if connections.socket != nil {
        _, err = io.Copy(connections.socket, r)
      } else {
        _, err = io.Copy(os.Stdout, r)
      }

      if err != nil {
        log.Printf("Error while writing to socket: %s", err)
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
