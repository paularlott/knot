package proxy

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/util"
	"github.com/spf13/viper"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
)

func HandleWSProxyServer(w http.ResponseWriter, r *http.Request) {
  ws := util.UpgradeToWS(w, r);
  if ws == nil {
    log.Error().Msg("ws: error while upgrading to websocket")
    return
  }

  dns := viper.GetString("server.nameserver")
  host := chi.URLParam(r, "host")
  port := chi.URLParam(r, "port")

  // If port is 0 then use SRV lookup to find port
  if port == "0" {
    var err error
    host, port, err = util.GetTargetFromSRV(host, dns)
    if err != nil {
      log.Error().Msgf("ws: error while looking up SRV record: %s", err.Error())
      ws.Close()
      return
    }

    log.Info().Msgf("ws: proxying to %s via %s:%s", chi.URLParam(r, "host"), host, port)
  } else {
    log.Info().Msgf("ws: proxying to %s:%s", host, port)
  }

  // Open tcp connection to target
  tcpConn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("ws: error while dialing %s:%s: %s", host, port, err.Error())
    return
  }

  copier := NewCopier(tcpConn, ws)
  go copier.Run()
}
