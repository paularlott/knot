package proxy

import (
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/util"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

func HandleWSProxyServer(w http.ResponseWriter, r *http.Request, ws *websocket.Conn, dns string) {
	vars := mux.Vars(r)
	host := vars["host"]
	port := vars["port"]

  // If port is 0 then use SRV lookup to find port
  if port == "0" {
    var err error
    host, port, err = util.GetTargetFromSRV(host, dns)
    if err != nil {
      log.Error().Msgf("Error while looking up SRV record: %s", err.Error())
      ws.Close()
      return
    }

    log.Info().Msgf("Proxying to %s via %s:%s", vars["host"], host, port)
  } else {
    log.Info().Msgf("Proxying to %s:%s", host, port)
  }

  // Open tcp connection to target
  tcpConn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Error().Msgf("Error while dialing %s:%s: %s", host, port, err.Error())
    return
  }

  copier := NewCopier(tcpConn, ws)
  go copier.Run()
}
