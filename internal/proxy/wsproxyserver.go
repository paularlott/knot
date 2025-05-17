package proxy

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/validate"

	"github.com/rs/zerolog/log"
)

func HandleWSProxyServer(w http.ResponseWriter, r *http.Request) {
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("ws: error while upgrading to websocket")
		return
	}

	host := r.PathValue("host")
	if !validate.Name(host) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	port := r.PathValue("port")
	portInt, err := strconv.Atoi(port)
	if err != nil || !validate.IsNumber(portInt, 0, 65535) {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// If port is 0 then use SRV lookup to find port
	if port == "0" {
		var err error
		hostPorts, err := util.LookupSRV(host)
		if err != nil {
			log.Error().Msgf("ws: error while looking up SRV record: %s", err.Error())
			ws.Close()
			return
		}

		host = (*hostPorts)[0].Host
		port = (*hostPorts)[0].Port
	}

	log.Info().Msgf("ws: proxying to %s:%s", host, port)

	// Open tcp connection to target
	tcpConn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10*time.Second)
	if err != nil {
		ws.Close()
		log.Error().Msgf("ws: error while dialing %s:%s: %s", host, port, err.Error())
		return
	}

	copier := util.NewCopier(tcpConn, ws)
	go copier.Run()
}
