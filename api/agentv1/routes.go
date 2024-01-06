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
  codeServerPort int
)

func Routes(cmd *cobra.Command) chi.Router {

  // Set a fake space key so that any calls fail
  middleware.AgentSpaceKey = uuid.New().String()

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
    codeServerPort = viper.GetInt("agent.port.code-server")
    if codeServerPort != 0 {
      log.Info().Msgf("Enabling proxy to code-server on port: %d", codeServerPort)
      router.HandleFunc("/code-server/*", agentProxyCodeServer);
    }

    // If allowing TCP ports then enable the proxy
    if len(AllowedPortMap) > 0 {
      log.Info().Msg("Enabling proxy for any TCP port")
      router.HandleFunc("/tcp/{port}/", agentProxyTCP);
    }

    if viper.GetBool("agent.enable-terminal") {
      log.Info().Msg("Enabling web terminal")
      router.HandleFunc("/terminal/{shell:^[a-z]+$}/", agentTerminal);
    }
  })


  // TODO Fix up everything below here

  if cmd.Flag("disable-http").Value.String() != "true" {
    log.Info().Msg("Enabling proxying of HTTP ports")
    router.HandleFunc("/http/{port}/*", agentProxyHTTP);
  }

  return router
}
