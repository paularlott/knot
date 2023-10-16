package proxy

import (
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gorilla/websocket"
)

func RunTCPForwarder(proxyServerURL string, listen string, service string, port int) {
  log.Printf("Listening on %s", listen)
  log.Printf("Forwarding to %s", service)

  // Build dial address
  dialURL := fmt.Sprintf("%s/forward-port/%s/%d", proxyServerURL, service, port)

	tcpConnection, err := net.Listen("tcp", listen)
	if err != nil {
    log.Fatal("Error while opening local port: ", err)
		os.Exit(1)
	}
	defer tcpConnection.Close()

  for {
    tcpConn, err := tcpConnection.Accept()
		if err != nil {
			log.Printf("Error: could not accept the connection: %s", err)
			continue
		}

    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
    if err != nil {
      tcpConn.Close()
      log.Fatal("Error while dialing:", err)
      os.Exit(1)
    }
    copier := NewCopier(tcpConn, wsConn)
    go copier.Run()
  }
}
