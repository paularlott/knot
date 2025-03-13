package agent_service_api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

func ListenAndServe() {
	log.Debug().Msgf("service_api: starting agent service api on port %d", viper.GetInt("agent.api_port"))

	go func() {

		router := http.NewServeMux()
		router.HandleFunc("POST /logs", handleLogMessage)
		router.HandleFunc("POST /gelf", handleGelf)
		router.HandleFunc("POST /loki/api/v1/push", handleLoki)
		router.HandleFunc("POST /api/space/description", handleDescription)

		// Run the http server
		server := &http.Server{
			Addr:         fmt.Sprintf(":%d", viper.GetInt("agent.api_port")),
			Handler:      router,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		}

		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal().Msgf("log sink: %v", err.Error())
		}
	}()
}
