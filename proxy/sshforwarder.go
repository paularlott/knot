package proxy

import (
	"fmt"
	"log"
	"os"

	"github.com/gorilla/websocket"
)

func RunSSHForwarder(proxyServerURL string, service string, port int) {
  log.Println("Connecting to proxy server at", proxyServerURL)

  // Build dial address
  dialURL := fmt.Sprintf("%s/forward-port/%s/%d", proxyServerURL, service, port)

  for {
    // Create websocket connection
    wsConn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
    if err != nil {
      log.Fatal("Error while dialing:", err)
      os.Exit(1)
    }

    log.Println("about to start copy process!!!")

    copier := NewCopier(nil, wsConn)
    copier.Run()
  }
}
