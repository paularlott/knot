package configwizard

import (
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// prefillConfig mirrors the subset of knot.toml the wizard can edit. Only
// keys that are present in the file override the base form, so a partial
// config keeps wizard defaults for everything it doesn't set.
type prefillConfig struct {
	Server struct {
		Listen        string `toml:"listen"`
		ListenAgent   string `toml:"listen_agent"`
		AgentEndpoint string `toml:"agent_endpoint"`
		URL           string `toml:"url"`
		Wildcard      string `toml:"wildcard_domain"`
		Timezone      string `toml:"timezone"`
		Encrypt       string `toml:"encrypt"`
		ListenTunnel  string `toml:"listen_tunnel"`
		TunnelDomain  string `toml:"tunnel_domain"`
		TunnelServer  string `toml:"tunnel_server"`

		Origin struct {
			Server string `toml:"server"`
		} `toml:"origin"`

		BadgerDB struct {
			Enabled bool   `toml:"enabled"`
			Path    string `toml:"path"`
		} `toml:"badgerdb"`

		MySQL struct {
			Enabled  bool   `toml:"enabled"`
			Host     string `toml:"host"`
			Port     int    `toml:"port"`
			User     string `toml:"user"`
			Password string `toml:"password"`
			Database string `toml:"database"`
		} `toml:"mysql"`

		Redis struct {
			Enabled  bool     `toml:"enabled"`
			Hosts    []string `toml:"hosts"`
			Password string   `toml:"password"`
		} `toml:"redis"`

		Nomad struct {
			Addr   string `toml:"addr"`
			Token  string `toml:"token"`
			DC     string `toml:"dc"`
			Region string `toml:"region"`
		} `toml:"nomad"`

		Docker struct {
			Host string `toml:"host"`
		} `toml:"docker"`

		Podman struct {
			Host string `toml:"host"`
		} `toml:"podman"`

		DNS struct {
			Enabled bool     `toml:"enabled"`
			Listen  string   `toml:"listen"`
			Records []string `toml:"records"`
		} `toml:"dns"`

		Chat struct {
			Enabled bool   `toml:"enabled"`
			Type    string `toml:"type"`
			APIKey  string `toml:"api_key"`
			BaseURL string `toml:"base_url"`
			// Deprecated aliases the server still honours when the
			// modern keys are absent.
			OpenAIAPIKey  string `toml:"openai_api_key"`
			OpenAIBaseURL string `toml:"openai_base_url"`
			Model         string `toml:"model"`
		} `toml:"chat"`

		TOTP struct {
			Enabled bool   `toml:"enabled"`
			Issuer  string `toml:"issuer"`
		} `toml:"totp"`

		UI struct {
			EnableGravatar bool `toml:"enable_gravatar"`
		} `toml:"ui"`

		MCP struct {
			Enabled bool `toml:"enabled"`
		} `toml:"mcp"`

		// Audit settings live flat under [server] (server.audit_routing etc).
		AuditRouting   string `toml:"audit_routing"`
		AuditRetention int    `toml:"audit_retention"`
	} `toml:"server"`

	Log struct {
		Output struct {
			URL      string `toml:"url"`
			Format   string `toml:"format"`
			Token    string `toml:"token"`
			Username string `toml:"username"`
			Password string `toml:"password"`
		} `toml:"output"`
	} `toml:"log"`

	Resolver struct {
		Nameservers []string `toml:"nameservers"`
	} `toml:"resolver"`
}

// FormFromConfig overlays an existing knot.toml onto a base form. Missing
// keys keep the base values.
func FormFromConfig(base Form, path string) Form {
	data, err := os.ReadFile(path)
	if err != nil {
		return base
	}

	var cfg prefillConfig
	if _, err := toml.Decode(string(data), &cfg); err != nil {
		return base
	}

	s := cfg.Server

	// Derive the deployment shape from the config so editing doesn't
	// re-ask it — the existing fields drive the wizard. A preset from
	// desktop mode is authoritative.
	if base.Deployment == "" {
		switch {
		case s.Origin.Server != "", s.BadgerDB.Enabled && strings.Contains(s.Wildcard, "knot.internal"):
			base.Deployment = "desktop"
		case s.Nomad.Addr != "":
			base.Deployment = "cluster"
		default:
			base.Deployment = "standalone"
		}
	}
	if s.Listen != "" {
		base.Listen = s.Listen
	}
	if s.ListenAgent != "" {
		base.ListenAgent = s.ListenAgent
	}
	if s.AgentEndpoint != "" {
		base.AgentEndpoint = s.AgentEndpoint
	}
	if s.URL != "" {
		base.URL = s.URL
	}
	if s.Wildcard != "" {
		base.WildcardDomain = s.Wildcard
	}
	if s.Timezone != "" {
		base.Timezone = s.Timezone
	}
	if s.Encrypt != "" {
		base.EncryptionKey = s.Encrypt
	}
	if s.ListenTunnel != "" {
		base.TunnelEnabled = true
		base.TunnelListen = s.ListenTunnel
		base.TunnelDomain = s.TunnelDomain
		base.TunnelServer = s.TunnelServer
	}

	// Determine the primary database backend. Redis alongside another
	// backend is session storage, not the primary.
	switch {
	case s.BadgerDB.Enabled:
		base.DBType = "badger"
		base.BadgerPath = s.BadgerDB.Path
	case s.MySQL.Enabled:
		base.DBType = "mysql"
		base.MySQLHost, base.MySQLPort = s.MySQL.Host, s.MySQL.Port
		base.MySQLUser, base.MySQLPassword, base.MySQLDatabase = s.MySQL.User, s.MySQL.Password, s.MySQL.Database
	case s.Redis.Enabled:
		base.DBType = "redis"
	}
	if s.Redis.Enabled && base.DBType != "redis" {
		base.SessionRedis = true
	}
	if s.Redis.Enabled {
		base.RedisHosts = s.Redis.Hosts
		base.RedisPassword = s.Redis.Password
	}
	if base.MySQLPort == 0 {
		base.MySQLPort = 3306
	}

	base.NomadAddr, base.NomadToken = s.Nomad.Addr, s.Nomad.Token
	base.NomadDC, base.NomadRegion = s.Nomad.DC, s.Nomad.Region
	base.DockerHost = s.Docker.Host
	base.PodmanHost = s.Podman.Host
	base.DNSEnabled, base.DNSListen = s.DNS.Enabled, s.DNS.Listen
	base.DNSRecords = s.DNS.Records
	base.ChatEnabled, base.ChatType = s.Chat.Enabled, s.Chat.Type
	base.ChatAPIKey, base.ChatBaseURL, base.ChatModel = s.Chat.APIKey, s.Chat.BaseURL, s.Chat.Model
	// Fall back to the deprecated aliases, mirroring how the server
	// resolves them, so editing an older config doesn't drop the values.
	if base.ChatAPIKey == "" {
		base.ChatAPIKey = s.Chat.OpenAIAPIKey
	}
	if base.ChatBaseURL == "" {
		base.ChatBaseURL = s.Chat.OpenAIBaseURL
	}
	if base.ChatType == "" {
		base.ChatType = "openai"
	}
	base.TotpEnabled, base.TotpIssuer = s.TOTP.Enabled, s.TOTP.Issuer
	if base.TotpIssuer == "" {
		base.TotpIssuer = "Knot"
	}
	base.EnableGravatar = s.UI.EnableGravatar
	base.EnableMCP = s.MCP.Enabled
	if s.AuditRouting != "" {
		base.AuditRouting = s.AuditRouting
	}
	if s.AuditRetention > 0 {
		base.AuditRetention = s.AuditRetention
	}
	if cfg.Log.Output.URL != "" {
		base.LogOutputEnabled = true
		base.LogOutputURL = cfg.Log.Output.URL
	}
	if cfg.Log.Output.Format != "" {
		base.LogOutputFormat = cfg.Log.Output.Format
	}
	base.LogOutputToken = cfg.Log.Output.Token
	base.LogOutputUsername = cfg.Log.Output.Username
	base.LogOutputPassword = cfg.Log.Output.Password
	if len(cfg.Resolver.Nameservers) > 0 {
		base.Nameservers = strings.Join(cfg.Resolver.Nameservers, ", ")
	}

	return base
}
