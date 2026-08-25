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
	Deployment string `json:"deployment"`

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

	DBType       string `json:"db_type"`
	SessionRedis bool   `json:"session_redis"`
	Nameservers  string `json:"nameservers"`

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

	DNSEnabled bool     `json:"dns_enabled"`
	DNSListen  string   `json:"dns_listen"`
	DNSRecords []string `json:"dns_records"`

	ChatEnabled bool   `json:"chat_enabled"`
	ChatType    string `json:"chat_type"`
	ChatAPIKey  string `json:"chat_api_key"`
	ChatBaseURL string `json:"chat_base_url"`
	ChatModel   string `json:"chat_model"`

	TunnelEnabled bool   `json:"tunnel_enabled"`
	TunnelListen  string `json:"tunnel_listen"`
	TunnelDomain  string `json:"tunnel_domain"`
	TunnelServer  string `json:"tunnel_server"`

	TotpEnabled        bool   `json:"totp_enabled"`
	TotpIssuer         string `json:"totp_issuer"`
	AuthIPRateLimiting bool   `json:"auth_ip_rate_limiting"`

	AuthRateLimitAttempts int `json:"auth_rate_limit_attempts"`
	AuthRateLimitWindow   int `json:"auth_rate_limit_window"`
	AuthRateLimitBlock    int `json:"auth_rate_limit_block"`

	ClusterAdvertiseAddr string `json:"cluster_advertise_addr"`
	ClusterKey           string `json:"cluster_key"`
	ClusterPeers         string `json:"cluster_peers"`
	AllowLeafNodes       bool   `json:"allow_leaf_nodes"`

	EnableGravatar bool `json:"enable_gravatar"`

	LogOutputEnabled  bool   `json:"log_output_enabled"`
	LogOutputURL      string `json:"log_output_url"`
	LogOutputFormat   string `json:"log_output_format"`
	LogOutputToken    string `json:"log_output_token"`
	LogOutputUsername string `json:"log_output_username"`
	LogOutputPassword string `json:"log_output_password"`
	AuditRouting      string `json:"audit_routing"`
	AuditRetention    int    `json:"audit_retention"`
	AuditFileOps      bool   `json:"audit_file_operations"`
	AuditSessions     bool   `json:"audit_space_sessions"`
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
