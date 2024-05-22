package agentv1

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

var (
	TcpPortMap map[string]string
)

func agentProxyTCP(w http.ResponseWriter, r *http.Request) {
	port := chi.URLParam(r, "port")

	log.Debug().Msgf("proxy of tcp port %s", port)

	// Check port is in the list of allowed ports
	if _, ok := TcpPortMap[port]; !ok {
		log.Error().Msgf("proxy of tcp port %s is not allowed", port)
		w.WriteHeader(http.StatusForbidden)
		return
	}

	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Open tcp connection to target
	dial := net.JoinHostPort("127.0.0.1", port)
	tcpConn, err := net.DialTimeout("tcp", dial, 10*time.Second)
	if err != nil {
		ws.Close()
		log.Error().Msgf("error while dialing %s: %s", dial, err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	copier := util.NewCopier(tcpConn, ws)
	go copier.Run()
}
