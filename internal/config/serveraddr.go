package config

import (
	"regexp"
	"strings"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/log"
)

type ServerAddr struct {
	HttpServer string
	WsServer   string
	ApiToken   string
}

// Read the server configuration information and generate the websocket address
func GetServerAddr(alias string, cmd *cli.Command) *ServerAddr {
	flags := &ServerAddr{}

	re := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-]{1,19}$`)
	if !re.MatchString(alias) {
		log.Fatal("Alias must be alphanumeric and can contain -, must start with a letter and be 20 characters or less")
	}

	// Use the server and token flags if given, else use the alias
	if cmd.HasFlag("server") && cmd.HasFlag("token") {
		flags.HttpServer = cmd.GetString("server")
		flags.ApiToken = cmd.GetString("token")
	} else {
		v, exists := cmd.ConfigFile.GetValue("client.connection." + alias + ".server")
		if exists {
			flags.HttpServer = v.(string)
		}

		v, exists = cmd.ConfigFile.GetValue("client.connection." + alias + ".token")
		if exists {
			flags.ApiToken = v.(string)
		}
	}

	// If flags.server empty then throw and error
	if flags.HttpServer == "" {
		log.Fatal("Missing knot server address")
	}

	if flags.ApiToken == "" {
		log.Fatal("Missing knot API token")
	}

	if !strings.HasPrefix(flags.HttpServer, "http://") && !strings.HasPrefix(flags.HttpServer, "https://") {
		flags.HttpServer = "https://" + flags.HttpServer
	}

	// Fix up the address to a websocket address
	flags.HttpServer = strings.TrimSuffix(flags.HttpServer, "/")
	flags.WsServer = "ws" + flags.HttpServer[4:]

	return flags
}
