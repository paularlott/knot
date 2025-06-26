package proxy

import (
	"io"
	"net/http"
	"sync"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/wsconn"

	"github.com/rs/zerolog/log"
)

func proxyAgentPort(w http.ResponseWriter, r *http.Request, agentSession *agent_server.Session, port uint16) {

	// Open a new stream to the agent
	stream, err := agentSession.MuxSession.Open()
	if err != nil {
		log.Debug().Err(err).Msg("Error opening stream to agent")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer stream.Close()

	// Write the command
	if err := msg.WriteCommand(stream, msg.CmdProxyTCPPort); err != nil {
		log.Debug().Err(err).Msg("Error writing command")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if err := msg.WriteMessage(stream, &msg.TcpPort{
		Port: port,
	}); err != nil {
		log.Debug().Err(err).Msg("Error writing message")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Upgrade the connection to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Debug().Msg("Error upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	conn := wsconn.New(ws)

	// copy data between code server and server
	var once sync.Once
	closeConn := func() {
		conn.Close()
	}

	// Copy from client to tunnel
	go func() {
		_, _ = io.Copy(conn, stream)
		once.Do(closeConn)
	}()

	// Copy from tunnel to client
	_, _ = io.Copy(stream, conn)
	once.Do(closeConn)
}
