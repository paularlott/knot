package agentv1

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/paularlott/knot/middleware"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
  codeServerPort string
  sshPort string
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
  })


  // TODO Fix up everything below here
  // If code server port given the enable the proxy
  codeServerPort = cmd.Flag("code-server").Value.String()
  if codeServerPort != "0" {
    log.Info().Msgf("Proxying to code-server on port: %s", codeServerPort)
    router.HandleFunc("/code-server/*", proxyCodeServer);
  }

  // If ssh port given the enable the proxy
  sshPort = cmd.Flag("ssh").Value.String()
  if sshPort != "0" {
    log.Info().Msgf("Proxying to SSH server on port: %s", sshPort)
    router.HandleFunc("/ssh/", proxySSH);
  }

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
