package agent_client

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"

	"github.com/rs/zerolog/log"
)

func ProxyTcp(stream net.Conn, port string) {
	conn, err := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port))
	if err != nil {
		log.Error().Err(err).Msg("agent: failed to connect to code server")
		return
	}
	defer conn.Close()

	// copy data between code server and server
	go io.Copy(conn, stream)
	io.Copy(stream, conn)
}

func ProxyTcpTls(stream net.Conn, port string, serverName string) {
	conn, err := tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%s", port), &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         serverName,
	})
	if err != nil {
		log.Error().Err(err).Msg("agent: failed to connect to code server")
		return
	}
	defer conn.Close()

	// copy data between code server and server
	go io.Copy(conn, stream)
	io.Copy(stream, conn)
}
