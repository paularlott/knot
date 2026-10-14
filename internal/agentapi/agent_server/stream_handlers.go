package agent_server

import (
	"net"
	"sync"
)

// Commands an agent sends that the server answers through a registered
// handler rather than one built in here, such as personal secrets in Knot
// Pro. The wire definitions live in the msg package in every edition, so
// agents and servers stay protocol-compatible; a command no handler is
// registered for is unknown, and its stream is closed.

var (
	streamHandlersMu sync.RWMutex
	streamHandlers   = map[byte]func(stream net.Conn, session *Session){}
)

// RegisterStreamHandler answers cmd from agents with handler, which reads
// the request from the stream and writes the reply.
func RegisterStreamHandler(cmd byte, handler func(stream net.Conn, session *Session)) {
	streamHandlersMu.Lock()
	defer streamHandlersMu.Unlock()
	streamHandlers[cmd] = handler
}

// HasStreamHandler reports whether cmd is answered.
func HasStreamHandler(cmd byte) bool {
	return streamHandler(cmd) != nil
}

func streamHandler(cmd byte) func(stream net.Conn, session *Session) {
	streamHandlersMu.RLock()
	defer streamHandlersMu.RUnlock()
	return streamHandlers[cmd]
}
