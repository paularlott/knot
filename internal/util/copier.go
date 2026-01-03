//
// Copies data between a websocket and stdin / stdout / socket.
//

package util

import (
	"errors"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/paularlott/knot/internal/log"
)

type copierConnections struct {
	socket       net.Conn
	wsConnection *websocket.Conn
	closed       bool
	closeMutex   sync.Mutex
}

func NewCopier(socket net.Conn, wsConnection *websocket.Conn) *copierConnections {
	return &copierConnections{
		socket:       socket,
		wsConnection: wsConnection,
	}
}

func (connections *copierConnections) Run() error {
	logger := log.WithGroup("copier")
	done := make(chan struct{}, 2)

	// Copy tcp to websocket
	go func() {
		defer func() {
			connections.close()
			done <- struct{}{}
		}()

		var n int
		var err error

		buf := make([]byte, 32*1024)

		for {
			// Read data from the socket / stdin
			if connections.socket != nil {
				connections.socket.SetReadDeadline(time.Now().Add(10 * time.Second))
				n, err = connections.socket.Read(buf)
			} else {
				n, err = os.Stdin.Read(buf)
			}

			if err != nil {
				if os.IsTimeout(err) {
					continue
				}
				unwrappedErr := errors.Unwrap(err)
				if err != io.EOF && unwrappedErr != nil && unwrappedErr.Error() != "use of closed network connection" {
					logger.WithError(err).Error("error reading from socket:")
				}
				return
			}

			// Write data to the websocket if we read any
			if n > 0 {
				if err := connections.wsConnection.WriteMessage(websocket.BinaryMessage, buf[:n]); err != nil {
					logger.WithError(err).Error("error writing to websocket:")
					return
				}
			}
		}
	}()

	// Copy websocket to tcp
	go func() {
		defer func() {
			connections.close()
			done <- struct{}{}
		}()

		for {
			// Read data from the websocket
			mt, r, err := connections.wsConnection.NextReader()
			if err != nil {
				unwrappedErr := errors.Unwrap(err)
				if unwrappedErr != nil && unwrappedErr.Error() != "use of closed network connection" {
					logger.WithError(err).Error("error reading from websocket:")
				}
				return
			}
			if mt != websocket.BinaryMessage {
				logger.Error("received unsupported websocket message type")
				return
			}

			// Write data to the socket / stdout
			if connections.socket != nil {
				_, err = io.Copy(connections.socket, r)
			} else {
				_, err = io.Copy(os.Stdout, r)
			}

			if err != nil {
				logger.WithError(err).Error("error while writing to socket:")
				return
			}
		}
	}()

	<-done
	<-done
	return nil
}

func (connections *copierConnections) close() {
	connections.closeMutex.Lock()
	defer connections.closeMutex.Unlock()
	
	if connections.closed {
		return
	}
	connections.closed = true
	
	connections.wsConnection.WriteControl(websocket.CloseMessage, []byte{}, time.Now().Add(10*time.Second))
	connections.wsConnection.Close()

	if connections.socket != nil {
		connections.socket.Close()
	}
}
