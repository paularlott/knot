package agentcmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/dnsproxy"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	AgentCmd.Flags().StringP("endpoint", "", "", "The address of the server to connect to.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_AGENT_ENDPOINT environment variable if set.")
	AgentCmd.Flags().StringP("space-id", "", "", "The ID of the space the agent is providing.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SPACEID environment variable if set.")
	AgentCmd.Flags().StringSliceP("nameservers", "", []string{}, "The address of the nameserver to use for SRV lookups, can be given multiple times (default use system resolver).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_NAMESERVERS environment variable if set.")
	AgentCmd.Flags().IntP("code-server-port", "", 0, "The port code-server is running on.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_CODE_SERVER_PORT environment variable if set.")
	AgentCmd.Flags().IntP("ssh-port", "", 0, "The port sshd is running on.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SSH_PORT environment variable if set.")
	AgentCmd.Flags().StringSliceP("tcp-port", "", []string{}, "Can be specified multiple times to give the list of TCP ports to be exposed to the client.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TCP_PORT environment variable if set.")
	AgentCmd.Flags().StringSliceP("http-port", "", []string{}, "Can be specified multiple times to give the list of http ports to be exposed via the web interface.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HTTP_PORT environment variable if set.")
	AgentCmd.Flags().StringSliceP("https-port", "", []string{}, "Can be specified multiple times to give the list of https ports to be exposed via the web interface.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_HTTPS_PORT environment variable if set.")
	AgentCmd.Flags().BoolP("update-authorized-keys", "", true, "If given then the agent will update the authorized_keys file with the SSH public key of the user.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_UPDATE_AUTHORIZED_KEYS environment variable if set.")
	AgentCmd.Flags().BoolP("enable-terminal", "", true, "If given then the agent will enable the web terminal.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ENABLE_TERMINAL environment variable if set.")
	AgentCmd.Flags().IntP("vnc-http-port", "", 0, "The port to use for VNC over HTTP.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_VNC_HTTP_PORT environment variable if set.")
	AgentCmd.Flags().StringP("service-password", "", "", "The password to use for the agent.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_SERVICE_PASSWORD environment variable if set.")
	AgentCmd.Flags().StringP("vscode-tunnel", "", "", "The name of the screen running the Visual Studio Code tunnel.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_VSCODE_TUNNEL environment variable if set.")
	AgentCmd.Flags().StringP("vscode-binary", "", "~/.local/bin/code", "The path to the code binary to use for the agent (defaults \"~/.local/bin/code\").\nOverrides the "+config.CONFIG_ENV_PREFIX+"_VSCODE_BINARY environment variable if set.")
	AgentCmd.Flags().StringP("advertise-addr", "", "", "The address to advertise to the server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_ADVERTISE_ADDR environment variable if set.")

	// TLS
	AgentCmd.Flags().StringP("cert-file", "", "", "The file with the PEM encoded certificate to use for the agent.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_CERT_FILE environment variable if set.")
	AgentCmd.Flags().StringP("key-file", "", "", "The file with the PEM encoded key to use for the agent.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_KEY_FILE environment variable if set.")
	AgentCmd.Flags().BoolP("use-tls", "", true, "Enable TLS.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_USE_TLS environment variable if set.")
	AgentCmd.Flags().BoolP("tls-skip-verify", "", true, "Skip TLS verification when talking to server.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY environment variable if set.")

	// DNS Forwarding
	AgentCmd.Flags().StringP("dns-listen", "", "", "The address and port to listen on for DNS requests (defaults to disabled).\nOverrides the "+config.CONFIG_ENV_PREFIX+"_DNS_LISTEN environment variable if set.")
	AgentCmd.Flags().Uint16P("dns-refresh-max-age", "", 180, "If a cached entry has been used within this number of seconds of it expiring then auto refresh.\nOverrides the "+config.CONFIG_ENV_PREFIX+"_MAX_AGE environment variable if set.")
}

var AgentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the knot agent",
	Long: `Start the knot agent and listen for incoming connections.

The agent will listen on the port specified by the --listen flag and proxy requests to the code-server instance running on the host.`,
	Args: cobra.NoArgs,
	PreRun: func(cmd *cobra.Command, args []string) {
		viper.BindPFlag("agent.endpoint", cmd.Flags().Lookup("endpoint"))
		viper.BindEnv("agent.endpoint", config.CONFIG_ENV_PREFIX+"_AGENT_ENDPOINT")

		viper.BindPFlag("agent.space_id", cmd.Flags().Lookup("space-id"))
		viper.BindEnv("agent.space_id", config.CONFIG_ENV_PREFIX+"_SPACEID")

		viper.BindPFlag("agent.port.code_server", cmd.Flags().Lookup("code-server-port"))
		viper.BindEnv("agent.port.code_server", config.CONFIG_ENV_PREFIX+"_CODE_SERVER_PORT")
		viper.SetDefault("agent.port.code_server", "0")

		viper.BindPFlag("agent.port.vnc_http", cmd.Flags().Lookup("vnc-http-port"))
		viper.BindEnv("agent.port.vnc_http", config.CONFIG_ENV_PREFIX+"_VNC_HTTP_PORT")
		viper.SetDefault("agent.port.vnc_http", "0")

		viper.BindPFlag("agent.port.ssh", cmd.Flags().Lookup("ssh-port"))
		viper.BindEnv("agent.port.ssh", config.CONFIG_ENV_PREFIX+"_SSH_PORT")
		viper.SetDefault("agent.port.ssh", "0")

		viper.BindPFlag("agent.port.tcp_port", cmd.Flags().Lookup("tcp-port"))
		viper.BindEnv("agent.port.tcp_port", config.CONFIG_ENV_PREFIX+"_TCP_PORT")

		viper.BindPFlag("agent.port.http_port", cmd.Flags().Lookup("http-port"))
		viper.BindEnv("agent.port.http_port", config.CONFIG_ENV_PREFIX+"_HTTP_PORT")

		viper.BindPFlag("agent.port.https_port", cmd.Flags().Lookup("https-port"))
		viper.BindEnv("agent.port.https_port", config.CONFIG_ENV_PREFIX+"_HTTPS_PORT")

		viper.BindPFlag("agent.update_authorized_keys", cmd.Flags().Lookup("update-authorized-keys"))
		viper.BindEnv("agent.update_authorized_keys", config.CONFIG_ENV_PREFIX+"_UPDATE_AUTHORIZED_KEYS")
		viper.SetDefault("agent.update_authorized_keys", true)

		viper.BindPFlag("agent.enable_terminal", cmd.Flags().Lookup("enable-terminal"))
		viper.BindEnv("agent.enable_terminal", config.CONFIG_ENV_PREFIX+"_ENABLE_TERMINAL")
		viper.SetDefault("agent.enable_terminal", true)

		viper.BindPFlag("agent.service_password", cmd.Flags().Lookup("service-password"))
		viper.BindEnv("agent.service_password", config.CONFIG_ENV_PREFIX+"_SERVICE_PASSWORD")
		viper.SetDefault("agent.service_password", "")

		viper.BindPFlag("agent.vscode_tunnel", cmd.Flags().Lookup("vscode-tunnel"))
		viper.BindEnv("agent.vscode_tunnel", config.CONFIG_ENV_PREFIX+"_VSCODE_TUNNEL")
		viper.SetDefault("agent.vscode_tunnel", "")

		viper.BindPFlag("agent.vscode_binary", cmd.Flags().Lookup("vscode-binary"))
		viper.BindEnv("agent.vscode_binary", config.CONFIG_ENV_PREFIX+"_VSCODE_BINARY")
		viper.SetDefault("agent.vscode_binary", "~/.local/bin/code")

		viper.BindPFlag("agent.advertise_addr", cmd.Flags().Lookup("advertise-addr"))
		viper.BindEnv("agent.advertise_addr", config.CONFIG_ENV_PREFIX+"_ADVERTISE_ADDR")
		viper.SetDefault("agent.advertise_addr", "")

		// TLS
		viper.BindPFlag("agent.tls.cert_file", cmd.Flags().Lookup("cert-file"))
		viper.BindEnv("agent.tls.cert_file", config.CONFIG_ENV_PREFIX+"_CERT_FILE")
		viper.SetDefault("agent.tls.cert_file", "")

		viper.BindPFlag("agent.tls.key_file", cmd.Flags().Lookup("key-file"))
		viper.BindEnv("agent.tls.key_file", config.CONFIG_ENV_PREFIX+"_KEY_FILE")
		viper.SetDefault("agent.tls.key_file", "")

		viper.BindPFlag("agent.tls.use_tls", cmd.Flags().Lookup("use-tls"))
		viper.BindEnv("agent.tls.use_tls", config.CONFIG_ENV_PREFIX+"_USE_TLS")
		viper.SetDefault("agent.tls.use_tls", true)

		viper.BindPFlag("tls_skip_verify", cmd.Flags().Lookup("tls-skip-verify"))
		viper.BindEnv("tls_skip_verify", config.CONFIG_ENV_PREFIX+"_TLS_SKIP_VERIFY")
		viper.SetDefault("tls_skip_verify", true)

		// DNS
		viper.BindPFlag("resolver.nameservers", cmd.Flags().Lookup("nameserver"))
		viper.BindEnv("resolver.nameservers", config.CONFIG_ENV_PREFIX+"_NAMESERVERS")

		viper.BindPFlag("dns.listen", cmd.Flags().Lookup("dns-listen"))
		viper.BindEnv("dns.listen", config.CONFIG_ENV_PREFIX+"_DNS_LISTEN")

		viper.BindPFlag("dns.refresh_max_age", cmd.Flags().Lookup("dns-refresh-max-age"))
		viper.BindEnv("dns.refresh_max_age", config.CONFIG_ENV_PREFIX+"_MAX_AGE")
		viper.SetDefault("dns.refresh_max_age", 180)
	},
	Run: func(cmd *cobra.Command, args []string) {
		serverAddr := viper.GetString("agent.endpoint")
		spaceId := viper.GetString("agent.space_id")

		// Check address given and valid URL
		if serverAddr == "" {
			log.Fatal().Msg("server address is required")
		}

		// Check the key is given
		if len(spaceId) != 36 {
			log.Fatal().Msg("space-id is required and must be a valid space ID")
		}

		// Start the DNS forwarder if enabled
		if viper.GetString("dns.listen") != "" {
			dnsproxy := dnsproxy.NewDNSProxy()
			go dnsproxy.RunServer()
		}

		// Open agent connection to the server
		agent_client.ConnectAndServe(serverAddr, spaceId)
		go agent_client.ReportState(spaceId)

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)

		// Block until we receive our signal.
		<-c

		agent_client.Shutdown()
		fmt.Println("\r")
		log.Info().Msg("agent: shutdown")
		os.Exit(0)
	},
}
