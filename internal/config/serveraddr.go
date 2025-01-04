package config

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ServerAddr struct {
	HttpServer string
	WsServer   string
	ApiToken   string
}

// Read the server configuration information and generate the websocket address
func GetServerAddr() *ServerAddr {
	flags := &ServerAddr{}

	flags.HttpServer = viper.GetString("client.server")
	flags.ApiToken = viper.GetString("client.token")

	// If flags.server empty then throw and error
	if flags.HttpServer == "" {
		cobra.CheckErr("Missing proxy server address")
	}

	if flags.ApiToken == "" {
		cobra.CheckErr("Missing API token")
	}

	if !strings.HasPrefix(flags.HttpServer, "http://") && !strings.HasPrefix(flags.HttpServer, "https://") {
		flags.HttpServer = "https://" + flags.HttpServer
	}

	// Fix up the address to a websocket address
	flags.HttpServer = strings.TrimSuffix(flags.HttpServer, "/")
	flags.WsServer = "ws" + flags.HttpServer[4:]

	return flags
}
