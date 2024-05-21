package agentv1

import (
	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	codeServerPort    int
	vncHttpServerPort int
)

func Routes(cmd *cobra.Command) chi.Router {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	// Set a fake space key so that any calls fail
	middleware.AgentSpaceKey = id.String()

	router := chi.NewRouter()

	// Ping needs to be public so nomad can monitor the health of the agent
	router.Get("/ping", HandleAgentPing)

	// Group routes that require authentication
	router.Group(func(router chi.Router) {
		router.Use(middleware.AgentApiAuth)

		// If SSH port given
		if viper.GetInt("agent.port.ssh") > 0 {
			log.Info().Msg("Enabling update authorized keys")
			router.Post("/update-authorized-keys", HandleAgentUpdateAuthorizedKeys)
		}

		// If code server port given then enable the proxy
		codeServerPort = viper.GetInt("agent.port.code_server")
		if codeServerPort != 0 {
			log.Info().Msgf("Enabling proxy to code-server on port: %d", codeServerPort)
			router.HandleFunc("/code-server/*", agentProxyCodeServer)
		}

		// If VNC port given then enable the proxy
		vncHttpServerPort = viper.GetInt("agent.port.vnc_http")
		if vncHttpServerPort != 0 {
			log.Info().Msgf("Enabling proxy to HTTP VNC on port: %d", vncHttpServerPort)
			router.HandleFunc("/vnc/*", agentProxyVNCHttp)
		}

		// If allowing TCP ports then enable the proxy
		if len(TcpPortMap) > 0 {
			log.Info().Msg("Enabling proxy for TCP ports")
			router.HandleFunc("/tcp/{port}/", agentProxyTCP)
		}

		// If allowing HTTP ports then enable the proxy
		if len(HttpPortMap) > 0 || len(HttpsPortMap) > 0 {
			log.Info().Msg("Enabling proxy for HTTP ports")
			router.HandleFunc("/http/{port}/*", agentProxyHTTP)
		}

		if viper.GetBool("agent.enable_terminal") {
			log.Info().Msg("Enabling web terminal")
			router.HandleFunc("/terminal/{shell:^[a-z]+$}/", agentTerminal)
		}
	})

	return router
}
