package config

import (
	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/log"
)

type ServerConfig struct {
	Listen                    string
	ListenAgent               string
	URL                       string
	AgentEndpoint             string
	WildcardDomain            string
	HTMLPath                  string
	TemplatePath              string
	AgentPath                 string
	PrivateFilesPath          string
	PublicFilesPath           string
	DownloadPath              string
	DisableSpaceCreate        bool
	ListenTunnel              string
	TunnelDomain              string
	TunnelServer              string
	TerminalWebGL             bool
	EncryptionKey             string
	Zone                      string
	Hostname                  string
	Timezone                  string
	LeafNode                  bool
	AuthIPRateLimiting        bool
	LocalContainerRuntimePref []string
	Origin                    OriginConfig
	TOTP                      TOTPConfig
	UI                        UIConfig
	Cluster                   ClusterConfig
	MySQL                     MySQLConfig
	BadgerDB                  BadgerDBConfig
	Redis                     RedisConfig
	Audit                     AuditConfig
	Docker                    DockerConfig
	Podman                    PodmanConfig
	Nomad                     NomadConfig
	TLS                       TLSConfig
	MCP                       MCPConfig
	Chat                      ChatConfig
	SkillsPath                string
}

type OriginConfig struct {
	Server string
	Token  string
}

type TOTPConfig struct {
	Enabled bool
	Window  int
	Issuer  string
}

type UIConfig struct {
	HideSupportLinks   bool
	HideAPITokens      bool
	EnableGravatar     bool
	LogoURL            string
	LogoInvert         bool
	EnableBuiltinIcons bool
	Icons              []string
}

type ClusterConfig struct {
	Key            string
	AdvertiseAddr  string
	BindAddr       string
	Peers          []string
	AllowLeafNodes bool
	Compression    bool
}

type TLSConfig struct {
	CertFile    string
	KeyFile     string
	UseTLS      bool
	AgentUseTLS bool
	SkipVerify  bool
}

type MySQLConfig struct {
	Enabled               bool
	Host                  string
	Port                  int
	User                  string
	Password              string
	Database              string
	ConnectionMaxIdle     int
	ConnectionMaxOpen     int
	ConnectionMaxLifetime int
}

type BadgerDBConfig struct {
	Enabled bool
	Path    string
}

type RedisConfig struct {
	Enabled    bool
	Hosts      []string
	Password   string
	DB         int
	MasterName string
	KeyPrefix  string
}

type AuditConfig struct {
	Retention int
}

type DockerConfig struct {
	Host string
}

type PodmanConfig struct {
	Host string
}

type NomadConfig struct {
	Host  string
	Token string
}

type MCPConfig struct {
	Enabled bool
}

type ChatConfig struct {
	Enabled          bool
	OpenAIAPIKey     string
	OpenAIBaseURL    string
	Model            string
	MaxTokens        int
	Temperature      float32
	SystemPromptFile string
	SystemPrompt     string
	ReasoningEffort  string
	UIStyle          string
}

// Global configuration instance
var (
	serverConfig *ServerConfig
)

// SetServerConfig sets the global server configuration
func SetServerConfig(config *ServerConfig) {
	serverConfig = config
}

// GetServerConfig returns the global server configuration
func GetServerConfig() *ServerConfig {
	return serverConfig
}

const CONFIG_ENV_PREFIX = "KNOT"
const CONFIG_FILE = "knot.toml"
const CONFIG_DIR = "knot"

func InitCommonConfig(cmd *cli.Command) {
	logLevel := cmd.GetString("log-level")
	log.Configure(logLevel, "console", nil)

	dns.UpdateNameservers(cmd.GetStringSlice("nameservers"))
}
