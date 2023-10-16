package proxy

import (
	"log"
	"net"
	"net/http"
	"time"

	"github.com/paularlott/knot/util"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
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
      log.Println("Error while looking up SRV record:", err)
      ws.Close()
      return
    }

    log.Printf("Proxying to %s via %s:%s", vars["host"], host, port)
  } else {
    log.Printf("Proxying to %s:%s", host, port)
  }

  // Open tcp connection to target
  tcpConn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 10 * time.Second)
  if err != nil {
    ws.Close()
    log.Printf("Error while dialing %s:%s: %s", host, port, err)
    return
  }

  copier := NewCopier(tcpConn, ws)
  go copier.Run()
}
