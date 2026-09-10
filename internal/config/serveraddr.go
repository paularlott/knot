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

var aliasRE = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9\-]{1,19}$`)

// NewServerAddr normalises a server URL into a ServerAddr, adding the https
// scheme when missing, trimming any trailing slash, and deriving the
// websocket address.
func NewServerAddr(server, token string) *ServerAddr {
	if !strings.HasPrefix(server, "http://") && !strings.HasPrefix(server, "https://") {
		server = "https://" + server
	}
	server = strings.TrimSuffix(server, "/")

	return &ServerAddr{
		HttpServer: server,
		WsServer:   "ws" + server[4:],
		ApiToken:   token,
	}
}

// LookupServerAddr resolves the client.connection.<alias> section of the
// config file into a ServerAddr. Unlike GetServerAddr it never exits: an
// invalid or unconfigured alias returns ok=false so callers can fall back to
// another connection source, e.g. the agentlink socket inside a space.
func LookupServerAddr(alias string, cmd *cli.Command) (*ServerAddr, bool) {
	if !aliasRE.MatchString(alias) || cmd.ConfigFile == nil {
		return nil, false
	}

	server, _ := cmd.ConfigFile.GetValue("client.connection." + alias + ".server")
	token, _ := cmd.ConfigFile.GetValue("client.connection." + alias + ".token")
	serverURL, _ := server.(string)
	apiToken, _ := token.(string)
	if serverURL == "" || apiToken == "" {
		return nil, false
	}

	return NewServerAddr(serverURL, apiToken), true
}

// Read the server configuration information and generate the websocket address
func GetServerAddr(alias string, cmd *cli.Command) *ServerAddr {
	if !aliasRE.MatchString(alias) {
		log.Fatal("Alias must be alphanumeric and can contain -, must start with a letter and be 20 characters or less")
	}

	// Use the server and token flags if given, else use the alias
	var server, token string
	if cmd.HasFlag("server") && cmd.HasFlag("token") {
		server = cmd.GetString("server")
		token = cmd.GetString("token")
	} else {
		v, exists := cmd.ConfigFile.GetValue("client.connection." + alias + ".server")
		if exists {
			server = v.(string)
		}

		v, exists = cmd.ConfigFile.GetValue("client.connection." + alias + ".token")
		if exists {
			token = v.(string)
		}
	}

	// If no server address then throw an error
	if server == "" {
		log.Fatal("Missing knot server address")
	}

	if token == "" {
		log.Fatal("Missing knot API token")
	}

	return NewServerAddr(server, token)
}
