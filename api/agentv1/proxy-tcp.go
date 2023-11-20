package agentv1

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/proxy"
	"github.com/paularlott/knot/util"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func proxyTCP(w http.ResponseWriter, r *http.Request) {
  ws := util.UpgradeToWS(w, r);
  if ws == nil {
    log.Error().Msg("proxyTCP: error while upgrading to websocket")
    return
  }

  port := chi.URLParam(r, "port")

  // Open tcp connection to target
  dial := net.JoinHostPort("127.0.0.1", port)
  tcpConn, err := net.DialTimeout("tcp", dial, 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("proxyTCP: error while dialing %s: %s", dial, err.Error())
    return
  }

  copier := proxy.NewCopier(tcpConn, ws)
  go copier.Run()
}
