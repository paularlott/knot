package agentv1

import (
	"net/http"

	"github.com/paularlott/knot/middleware"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
  codeServerPort int
  sshPort int
)

func Routes(cmd *cobra.Command) chi.Router {

  // Set a fake space key so that any calls fail
  middleware.AgentSpaceKey = uuid.New().String()

  router := chi.NewRouter()

  // Group routes that require authentication
  router.Group(func(router chi.Router) {
    router.Use(middleware.AgentApiAuth)

    // Core
    router.Get("/ping", HandleAgentPing)

    // If code server port given the enable the proxy
    codeServerPort = viper.GetInt("agent.port.code-server")
    if codeServerPort != 0 {
      log.Info().Msgf("Enabling proxy to code-server on port: %d", codeServerPort)
      router.HandleFunc("/code-server/*", agentProxyCodeServer);
    }

    // If ssh port given the enable the proxy
    sshPort = viper.GetInt("agent.port.ssh")
    if sshPort != 0 {
      log.Info().Msgf("Enabling proxy to SSH server on port: %d", sshPort)
      router.HandleFunc("/ssh/", agentProxySSH);
    }
  })


  // TODO Fix up everything below here


  if cmd.Flag("disable-tcp").Value.String() != "true" {
    log.Info().Msg("Enabling proxying of TCP ports")
    router.HandleFunc("/tcp/{port}/", proxyTCP);
  }

  if cmd.Flag("disable-http").Value.String() != "true" {
    log.Info().Msg("Enabling proxying of HTTP ports")
    router.HandleFunc("/http/{port}/*", proxyHTTP);
  }

  router.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
    log.Error().Msgf("Unknown request: %s", r.URL.Path)
    w.WriteHeader(http.StatusNotFound)
  })

  return router
}
