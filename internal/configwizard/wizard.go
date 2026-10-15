package configwizard

import (
	"os"
	"path/filepath"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/util"
	"github.com/paularlott/knot/internal/util/crypt"
)

type Form struct {
	// Deployment selects the step-1 preset card; empty means none selected.
	// "desktop" is the local-machine preset used by desktop mode.
	Deployment string `json:"deployment" msgpack:"deployment"`

	Listen         string `json:"listen" msgpack:"listen"`
	ListenAgent    string `json:"listen_agent" msgpack:"listen_agent"`
	URL            string `json:"url" msgpack:"url"`
	AgentEndpoint  string `json:"agent_endpoint" msgpack:"agent_endpoint"`
	WildcardDomain string `json:"wildcard_domain" msgpack:"wildcard_domain"`
	Timezone       string `json:"timezone" msgpack:"timezone"`

	NomadAddr   string `json:"nomad_addr" msgpack:"nomad_addr"`
	NomadToken  string `json:"nomad_token" msgpack:"nomad_token"`
	NomadDC     string `json:"nomad_dc" msgpack:"nomad_dc"`
	NomadRegion string `json:"nomad_region" msgpack:"nomad_region"`

	DockerHost string `json:"docker_host" msgpack:"docker_host"`
	PodmanHost string `json:"podman_host" msgpack:"podman_host"`

	DBType       string `json:"db_type" msgpack:"db_type"`
	SessionRedis bool   `json:"session_redis" msgpack:"session_redis"`
	Nameservers  string `json:"nameservers" msgpack:"nameservers"`

	MySQLHost     string `json:"mysql_host" msgpack:"mysql_host"`
	MySQLPort     int    `json:"mysql_port" msgpack:"mysql_port"`
	MySQLUser     string `json:"mysql_user" msgpack:"mysql_user"`
	MySQLPassword string `json:"mysql_password" msgpack:"mysql_password"`
	MySQLDatabase string `json:"mysql_database" msgpack:"mysql_database"`

	PostgresHost     string `json:"postgres_host" msgpack:"postgres_host"`
	PostgresPort     int    `json:"postgres_port" msgpack:"postgres_port"`
	PostgresUser     string `json:"postgres_user" msgpack:"postgres_user"`
	PostgresPassword string `json:"postgres_password" msgpack:"postgres_password"`
	PostgresDatabase string `json:"postgres_database" msgpack:"postgres_database"`

	RedisHosts    []string `json:"redis_hosts" msgpack:"redis_hosts"`
	RedisPassword string   `json:"redis_password" msgpack:"redis_password"`

	BadgerPath string `json:"badger_path" msgpack:"badger_path"`

	EncryptionKey string `json:"encryption_key" msgpack:"encryption_key"`
	EnableMCP     bool   `json:"enable_mcp" msgpack:"enable_mcp"`

	DNSEnabled bool     `json:"dns_enabled" msgpack:"dns_enabled"`
	DNSListen  string   `json:"dns_listen" msgpack:"dns_listen"`
	DNSRecords []string `json:"dns_records" msgpack:"dns_records"`

	ChatEnabled bool   `json:"chat_enabled" msgpack:"chat_enabled"`
	ChatType    string `json:"chat_type" msgpack:"chat_type"`
	ChatAPIKey  string `json:"chat_api_key" msgpack:"chat_api_key"`
	ChatBaseURL string `json:"chat_base_url" msgpack:"chat_base_url"`
	ChatModel   string `json:"chat_model" msgpack:"chat_model"`

	TunnelEnabled bool   `json:"tunnel_enabled" msgpack:"tunnel_enabled"`
	TunnelListen  string `json:"tunnel_listen" msgpack:"tunnel_listen"`
	TunnelDomain  string `json:"tunnel_domain" msgpack:"tunnel_domain"`
	TunnelServer  string `json:"tunnel_server" msgpack:"tunnel_server"`

	TotpEnabled        bool   `json:"totp_enabled" msgpack:"totp_enabled"`
	TotpIssuer         string `json:"totp_issuer" msgpack:"totp_issuer"`
	AuthIPRateLimiting bool   `json:"auth_ip_rate_limiting" msgpack:"auth_ip_rate_limiting"`

	AuthRateLimitAttempts int `json:"auth_rate_limit_attempts" msgpack:"auth_rate_limit_attempts"`
	AuthRateLimitWindow   int `json:"auth_rate_limit_window" msgpack:"auth_rate_limit_window"`
	AuthRateLimitBlock    int `json:"auth_rate_limit_block" msgpack:"auth_rate_limit_block"`

	ClusterAdvertiseAddr string `json:"cluster_advertise_addr" msgpack:"cluster_advertise_addr"`
	ClusterKey           string `json:"cluster_key" msgpack:"cluster_key"`
	ClusterPeers         string `json:"cluster_peers" msgpack:"cluster_peers"`
	AllowLeafNodes       bool   `json:"allow_leaf_nodes" msgpack:"allow_leaf_nodes"`

	EnableGravatar bool `json:"enable_gravatar" msgpack:"enable_gravatar"`

	LogOutputEnabled  bool   `json:"log_output_enabled" msgpack:"log_output_enabled"`
	LogOutputURL      string `json:"log_output_url" msgpack:"log_output_url"`
	LogOutputFormat   string `json:"log_output_format" msgpack:"log_output_format"`
	LogOutputToken    string `json:"log_output_token" msgpack:"log_output_token"`
	LogOutputUsername string `json:"log_output_username" msgpack:"log_output_username"`
	LogOutputPassword string `json:"log_output_password" msgpack:"log_output_password"`
	AuditRouting      string `json:"audit_routing" msgpack:"audit_routing"`
	AuditRetention    int    `json:"audit_retention" msgpack:"audit_retention"`
	AuditFileOps      bool   `json:"audit_file_operations" msgpack:"audit_file_operations"`
	AuditSessions     bool   `json:"audit_space_sessions" msgpack:"audit_space_sessions"`
}

func DefaultForm() Form {
	return Form{
		Listen:                "0.0.0.0:3000",
		ListenAgent:           "0.0.0.0:3010",
		URL:                   "",
		AgentEndpoint:         "",
		Timezone:              "",
		NomadAddr:             "http://127.0.0.1:4646",
		DockerHost:            "unix:///var/run/docker.sock",
		PodmanHost:            "unix:///run/podman/podman.sock",
		DBType:                "mysql",
		BadgerPath:            "./badgerdb/",
		MySQLHost:             "",
		MySQLPort:             3306,
		MySQLUser:             "",
		MySQLDatabase:         "knot",
		PostgresPort:          5432,
		PostgresDatabase:      "knot",
		RedisHosts:            nil,
		EncryptionKey:         crypt.CreateKey(),
		EnableMCP:             false,
		DNSEnabled:            false,
		DNSListen:             ":3053",
		ChatEnabled:           false,
		ChatType:              "openai",
		ChatBaseURL:           "",
		TunnelEnabled:         false,
		TotpEnabled:           false,
		TotpIssuer:            "Knot",
		EnableGravatar:        true,
		AuthIPRateLimiting:    true,
		AuthRateLimitAttempts: 10,
		AuthRateLimitWindow:   60,
		AuthRateLimitBlock:    300,
		AllowLeafNodes:        true,
		LogOutputFormat:       "ndjson",
		AuditRouting:          "internal",
		AuditRetention:        90,
		AuditFileOps:          false,
		AuditSessions:         false,
	}
}

// DesktopForm returns the local-machine preset used when desktop mode
// launches the wizard: embedded storage under ~/.knot, loopback listeners,
// a container-reachable agent endpoint template, and the built-in DNS
// server answering on knot.internal so local clients resolve space
// addresses without external DNS.
func DesktopForm() Form {
	form := DefaultForm()
	form.Deployment = "desktop"
	form.DBType = "badger"
	form.SessionRedis = false
	// A desktop is a single user on loopback — failed-login blocking would
	// only risk locking them out of their own machine.
	form.AuthIPRateLimiting = false
	if home, err := os.UserHomeDir(); err == nil {
		form.BadgerPath = filepath.Join(home, "."+config.CONFIG_DIR, "data")
	}
	form.Listen = "127.0.0.1:3000"
	form.URL = "http://127.0.0.1:3000"
	// Both the agent listener and the endpoint advertised to agents are
	// rendered to the host's IP at server startup — agents run in
	// containers and cannot reach the host via 127.0.0.1.
	form.ListenAgent = util.HostIPToken + ":3010"
	form.AgentEndpoint = util.HostIPToken + ":3010"
	form.WildcardDomain = "*.knot.internal"
	form.Timezone, _ = time.Now().Zone()
	form.DNSEnabled = true
	// Agents forward DNS directly to this listener (KNOT_SERVER_DNS points
	// at the agent-endpoint host), so it must bind the host IP — loopback
	// is unreachable from the containers.
	form.DNSListen = util.HostIPToken + ":3053"
	form.DNSRecords = []string{
		"A|knot.internal|" + util.HostIPToken + "|300",
		"A|*.knot.internal|" + util.HostIPToken + "|300",
	}
	form.NomadAddr = ""
	form.DockerHost = "unix:///var/run/docker.sock"
	form.PodmanHost = "unix:///run/podman/podman.sock"
	return form
}
