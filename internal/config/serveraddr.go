package config

import (
	"regexp"
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
func GetServerAddr(alias string) *ServerAddr {
	flags := &ServerAddr{}

	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-]{1,19}$`)
	if !re.MatchString(alias) {
		cobra.CheckErr("Alias must be alphanumeric and can contain -, must start with a letter and be 20 characters or less")
	}

	flags.HttpServer = viper.GetString("client." + alias + ".server")
	flags.ApiToken = viper.GetString("client." + alias + ".token")

	// If flags.server empty then throw and error
	if flags.HttpServer == "" {
		cobra.CheckErr("Missing knot server address")
	}

	if flags.ApiToken == "" {
		cobra.CheckErr("Missing knot API token")
	}

	if !strings.HasPrefix(flags.HttpServer, "http://") && !strings.HasPrefix(flags.HttpServer, "https://") {
		flags.HttpServer = "https://" + flags.HttpServer
	}

	// Fix up the address to a websocket address
	flags.HttpServer = strings.TrimSuffix(flags.HttpServer, "/")
	flags.WsServer = "ws" + flags.HttpServer[4:]

	return flags
}
