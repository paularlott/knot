package wsconn

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// A wrapper around a websocket.Conn that implements the net.Conn interface.
type Adapter struct {
	conn          *websocket.Conn
	readDeadline  time.Time
	writeDeadline time.Time
	reader        io.Reader
	readMutex     sync.Mutex
	writeMutex    sync.Mutex
}

func New(conn *websocket.Conn) *Adapter {
	return &Adapter{conn: conn}
}

func (ws *Adapter) Read(b []byte) (n int, err error) {
	ws.readMutex.Lock()
	defer ws.readMutex.Unlock()

	if ws.reader == nil {
		messageType, reader, err := ws.conn.NextReader()
		if err != nil {
			return 0, err
		}

		if messageType != websocket.BinaryMessage {
			// Consume the message and ignore it
			io.Copy(io.Discard, reader)

			return 0, nil
		}

		ws.reader = reader
	}

	bytesRead, err := ws.reader.Read(b)
	if err != nil {
		ws.reader = nil

		// EOF for the current Websocket frame, more will probably come so..
		if err == io.EOF {
			err = nil
		}
	}

	return bytesRead, err
}

func (ws *Adapter) Write(b []byte) (n int, err error) {
	ws.writeMutex.Lock()
	defer ws.writeMutex.Unlock()

	w, err := ws.conn.NextWriter(websocket.BinaryMessage)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	return w.Write(b)
}

func (ws *Adapter) Close() error {
	return ws.conn.Close()
}

func (ws *Adapter) LocalAddr() net.Addr {
	return ws.conn.LocalAddr()
}

func (ws *Adapter) RemoteAddr() net.Addr {
	return ws.conn.RemoteAddr()
}

func (ws *Adapter) SetDeadline(t time.Time) error {
	err := ws.SetReadDeadline(t)
	if err != nil {
		return err
	}
	return ws.SetWriteDeadline(t)
}

func (ws *Adapter) SetReadDeadline(t time.Time) error {
	ws.readDeadline = t
	return ws.conn.SetReadDeadline(t)
}

func (ws *Adapter) SetWriteDeadline(t time.Time) error {
	ws.writeDeadline = t
	return ws.conn.SetWriteDeadline(t)
}
