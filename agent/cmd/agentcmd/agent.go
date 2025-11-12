package agentcmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/agent/cmd/agentcmd/space"
	"github.com/paularlott/knot/internal/agent_service_api"
	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/agentlink"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/syslogd"

	"github.com/paularlott/cli"
	"github.com/paularlott/knot/internal/log"
)

var agentServerCmd = &cli.Command{
	Name:        "start",
	Usage:       "Start the knot agent",
	Description: `Start the knot agent and connect to the host server.`,
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:       "endpoint",
			Usage:      "The address of the server to connect to.",
			ConfigPath: []string{"agent.endpoint"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_AGENT_ENDPOINT"},
		},
		&cli.StringFlag{
			Name:       "space-id",
			Usage:      "The ID of the space the agent is providing.",
			ConfigPath: []string{"agent.space_id"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_SPACEID"},
		},
		&cli.IntFlag{
			Name:         "code-server-port",
			Usage:        "The port to run code-server on, set to 0 to disable.",
			ConfigPath:   []string{"agent.port.code_server"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_CODE_SERVER_PORT"},
			DefaultValue: 0xc0de,
		},
		&cli.IntFlag{
			Name:         "ssh-port",
			Usage:        "The port sshd is running on, set to 0 to disable ssh access.",
			ConfigPath:   []string{"agent.port.ssh"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_SSH_PORT"},
			DefaultValue: 22,
		},
		&cli.BoolFlag{
			Name:         "disable-terminal",
			Usage:        "Disable terminal access.",
			ConfigPath:   []string{"agent.disable_terminal"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DISABLE_TERMINAL"},
			DefaultValue: false,
		},
		&cli.BoolFlag{
			Name:         "disable-space-io",
			Usage:        "Disable space I/O operations (commands and file copy).",
			ConfigPath:   []string{"agent.disable_space_io"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_DISABLE_SPACE_IO"},
			DefaultValue: false,
		},
		&cli.StringSliceFlag{
			Name:       "tcp-port",
			Usage:      "Can be specified multiple times to give the list of TCP ports to be exposed to the client.",
			ConfigPath: []string{"agent.port.tcp_port"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_TCP_PORT"},
		},
		&cli.StringSliceFlag{
			Name:       "http-port",
			Usage:      "Can be specified multiple times to give the list of http ports to be exposed via the web interface.",
			ConfigPath: []string{"agent.port.http_port"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_HTTP_PORT"},
		},
		&cli.StringSliceFlag{
			Name:       "https-port",
			Usage:      "Can be specified multiple times to give the list of https ports to be exposed via the web interface.",
			ConfigPath: []string{"agent.port.https_port"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_HTTPS_PORT"},
		},
		&cli.BoolFlag{
			Name:         "update-authorized-keys",
			Usage:        "If given then the agent will update the authorized_keys file with the SSH public key of the user.",
			ConfigPath:   []string{"agent.update_authorized_keys"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_UPDATE_AUTHORIZED_KEYS"},
			DefaultValue: true,
		},
		&cli.IntFlag{
			Name:         "vnc-http-port",
			Usage:        "The port to use for VNC over HTTP.",
			ConfigPath:   []string{"agent.port.vnc_http"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_VNC_HTTP_PORT"},
			DefaultValue: 0,
		},
		&cli.StringFlag{
			Name:       "service-password",
			Usage:      "The password to use for the agent.",
			ConfigPath: []string{"agent.service_password"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_SERVICE_PASSWORD"},
		},
		&cli.StringFlag{
			Name:         "vscode-tunnel",
			Usage:        "The name of the screen running the Visual Studio Code tunnel, blank to disable.",
			ConfigPath:   []string{"agent.vscode_tunnel"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_VSCODE_TUNNEL"},
			DefaultValue: "vscodetunnel",
		},
		&cli.IntFlag{
			Name:         "syslog-port",
			Usage:        "The port to listen on for syslog messages, syslog is disabled if set to 0.",
			ConfigPath:   []string{"agent.syslog_port"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_SYSLOG_PORT"},
			DefaultValue: 514,
		},
		&cli.IntFlag{
			Name:         "api-port",
			Usage:        "The port to listen on for API requests and logs, disabled if set to 0.",
			ConfigPath:   []string{"agent.api_port"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_API_PORT"},
			DefaultValue: 12201,
		},
		// TLS flags
		&cli.StringFlag{
			Name:       "cert-file",
			Usage:      "The file with the PEM encoded certificate to use for the agent.",
			ConfigPath: []string{"agent.tls.cert_file"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_CERT_FILE"},
		},
		&cli.StringFlag{
			Name:       "key-file",
			Usage:      "The file with the PEM encoded key to use for the agent.",
			ConfigPath: []string{"agent.tls.key_file"},
			EnvVars:    []string{config.CONFIG_ENV_PREFIX + "_KEY_FILE"},
		},
		&cli.BoolFlag{
			Name:         "use-tls",
			Usage:        "Enable TLS.",
			ConfigPath:   []string{"agent.tls.use_tls"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_USE_TLS"},
			DefaultValue: true,
		},
		&cli.BoolFlag{
			Name:         "tls-skip-verify",
			Usage:        "Skip TLS verification when talking to server.",
			ConfigPath:   []string{"tls.skip_verify"},
			EnvVars:      []string{config.CONFIG_ENV_PREFIX + "_TLS_SKIP_VERIFY"},
			DefaultValue: true,
		},
	},
	Run: func(ctx context.Context, cmd *cli.Command) error {
		logger := log.WithGroup("agent")
		cfg := buildAgentConfig(cmd)

		// Check address given and valid URL
		if cfg.Endpoint == "" {
			log.Fatal("server address is required")
		}

		// Check the key is given
		if len(cfg.SpaceID) != 36 {
			log.Fatal("space-id is required and must be a valid space ID")
		}

		// Open agent connection to the server
		agentClient := agent_client.NewAgentClient(cfg.Endpoint, cfg.SpaceID)
		agentClient.ConnectAndServe()

		// Start the syslog server if enabled
		if cfg.SyslogPort > 0 {
			go syslogd.StartSyslogd(agentClient, cfg.SyslogPort)
		}

		// Start the http rest and log sink if enabled
		if cfg.APIPort > 0 {
			go agent_service_api.ListenAndServe(agentClient)
		}

		// Start the command socket
		agentlink.StartCommandSocket(agentClient)

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive our signal.
		<-c

		agentlink.StopCommandSocket()
		agentClient.Shutdown()
		fmt.Println("\r")
		logger.Info("shutdown")

		return nil
	},
}

func buildAgentConfig(cmd *cli.Command) *config.AgentConfig {
	agentCfg := &config.AgentConfig{
		Endpoint:             cmd.GetString("endpoint"),
		SpaceID:              cmd.GetString("space-id"),
		UpdateAuthorizedKeys: cmd.GetBool("update-authorized-keys"),
		ServicePassword:      cmd.GetString("service-password"),
		VSCodeTunnel:         cmd.GetString("vscode-tunnel"),
		SyslogPort:           cmd.GetInt("syslog-port"),
		APIPort:              cmd.GetInt("api-port"),
		DisableTerminal:      cmd.GetBool("disable-terminal"),
		DisableSpaceIO:       cmd.GetBool("disable-space-io"),
		Port: config.PortConfig{
			CodeServer: cmd.GetInt("code-server-port"),
			VNCHttp:    cmd.GetInt("vnc-http-port"),
			SSH:        cmd.GetInt("ssh-port"),
			TCPPorts:   cmd.GetStringSlice("tcp-port"),
			HTTPPorts:  cmd.GetStringSlice("http-port"),
			HTTPSPorts: cmd.GetStringSlice("https-port"),
		},
		TLS: config.TLSConfig{
			CertFile:   cmd.GetString("cert-file"),
			KeyFile:    cmd.GetString("key-file"),
			UseTLS:     cmd.GetBool("use-tls"),
			SkipVerify: cmd.GetBool("tls-skip-verify"),
		},
	}
	config.SetAgentConfig(agentCfg)

	return agentCfg
}

var AgentCmd = &cli.Command{
	Name:        "agent",
	Usage:       "Knot agent",
	Description: `Knot agent commands.`,
	Commands: []*cli.Command{
		agentServerCmd,
		space.SpaceNoteCmd,
		space.SpaceVarCmd,
		space.SpaceShutdownCmd,
		space.SpaceRestartCmd,
	},
}
