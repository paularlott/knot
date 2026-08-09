package configwizard

import (
	"github.com/paularlott/knot/internal/util/crypt"
)

type Form struct {
	Listen         string `json:"listen"`
	ListenAgent    string `json:"listen_agent"`
	URL            string `json:"url"`
	AgentEndpoint  string `json:"agent_endpoint"`
	WildcardDomain string `json:"wildcard_domain"`
	Timezone       string `json:"timezone"`

	NomadAddr   string `json:"nomad_addr"`
	NomadToken  string `json:"nomad_token"`
	NomadDC     string `json:"nomad_dc"`
	NomadRegion string `json:"nomad_region"`

	DockerHost string `json:"docker_host"`
	PodmanHost string `json:"podman_host"`

	DBType string `json:"db_type"`

	MySQLHost     string `json:"mysql_host"`
	MySQLPort     int    `json:"mysql_port"`
	MySQLUser     string `json:"mysql_user"`
	MySQLPassword string `json:"mysql_password"`
	MySQLDatabase string `json:"mysql_database"`

	RedisHosts    []string `json:"redis_hosts"`
	RedisPassword string   `json:"redis_password"`

	BadgerPath string `json:"badger_path"`

	EncryptionKey string `json:"encryption_key"`
	EnableMCP     bool   `json:"enable_mcp"`

	DNSEnabled bool   `json:"dns_enabled"`
	DNSListen  string `json:"dns_listen"`

	ChatEnabled bool   `json:"chat_enabled"`
	ChatType    string `json:"chat_type"`
	ChatAPIKey  string `json:"chat_api_key"`
	ChatBaseURL string `json:"chat_base_url"`
	ChatModel   string `json:"chat_model"`

	TunnelEnabled bool   `json:"tunnel_enabled"`
	TunnelListen  string `json:"tunnel_listen"`
	TunnelDomain  string `json:"tunnel_domain"`
	TunnelServer  string `json:"tunnel_server"`

	TotpEnabled bool   `json:"totp_enabled"`
	TotpIssuer  string `json:"totp_issuer"`

	EnableGravatar bool `json:"enable_gravatar"`
}

func DefaultForm() Form {
	return Form{
		Listen:         "0.0.0.0:3000",
		ListenAgent:    "0.0.0.0:3010",
		URL:            "",
		AgentEndpoint:  "",
		Timezone:       "",
		NomadAddr:      "http://127.0.0.1:4646",
		DockerHost:     "unix:///var/run/docker.sock",
		PodmanHost:     "unix:///run/podman/podman.sock",
		DBType:         "mysql",
		BadgerPath:     "./badgerdb/",
		MySQLHost:      "",
		MySQLPort:      3306,
		MySQLUser:      "",
		MySQLDatabase:  "knot",
		RedisHosts:     nil,
		EncryptionKey:  crypt.CreateKey(),
		EnableMCP:      false,
		DNSEnabled:     false,
		DNSListen:      ":3053",
		ChatEnabled:    false,
		ChatType:       "openai",
		ChatBaseURL:    "",
		TunnelEnabled:  false,
		TotpEnabled:    false,
		TotpIssuer:     "Knot",
		EnableGravatar: true,
	}
}
