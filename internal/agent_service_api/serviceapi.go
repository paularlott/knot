package agent_service_api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/agentapi/agent_client"
	"github.com/paularlott/knot/internal/config"

	"github.com/paularlott/knot/internal/log"
)

var agentClient *agent_client.AgentClient

func ListenAndServe(agent *agent_client.AgentClient) {
	cfg := config.GetAgentConfig()
	agentClient = agent

	log.Debug("service_api: starting agent service api", "port", cfg.APIPort)

	go func() {

		router := http.NewServeMux()
		router.HandleFunc("POST /logs", handleLogMessage)
		router.HandleFunc("POST /gelf", handleGelf)
		router.HandleFunc("POST /loki/api/v1/push", handleLoki)

		// Run the http server
		server := &http.Server{
			Addr:         fmt.Sprintf(":%d", cfg.APIPort),
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.WithError(err).Fatal("log sink")
		}
	}()
}
