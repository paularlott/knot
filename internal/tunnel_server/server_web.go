package tunnel_server

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"strings"

	"github.com/paularlott/knot/proxy"

	"github.com/rs/zerolog/log"
)

// Start a web server to listen for connections to tunnels, the left most part of the domain is the <username>--<tunnel name>
func ListenAndServe(listen string, tlsConfig *tls.Config) {
	log.Info().Msgf("tunnel: listening on %s", listen)

	go func() {

		mux := http.NewServeMux()

		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {

			// Split the domain into parts, 1st part is the tunnel name
			domainParts := strings.Split(r.Host, ".")
			if len(domainParts) < 1 {
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Get the tunnel session
			tunnelMutex.RLock()
			session, ok := tunnels[domainParts[0]]
			tunnelMutex.RUnlock()
			if !ok {
				log.Error().Msgf("tunnel: not found %s", domainParts[0])
				w.WriteHeader(http.StatusNotFound)
				return
			}

			// Open a new stream to the tunnel client
			stream, err := session.muxSession.Open()
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer stream.Close()

			// Write a byte with a value of 1 so the client knows this is a new connection
			_, err = stream.Write([]byte{1})
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			targetURL, err := url.Parse("http://127.0.0.1/")
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			httpProxy := proxy.CreateAgentReverseProxy(targetURL, stream, nil, r.Host)
			httpProxy.ServeHTTP(w, r)
		})

		if tlsConfig != nil {
			server := &http.Server{
				Addr:      listen,
				Handler:   mux,
				TLSConfig: tlsConfig,
			}
			err := server.ListenAndServeTLS("", "")
			if err != nil {
				log.Error().Err(err).Msg("tunnel: failed to start server")
			}
		} else {
			server := &http.Server{
				Addr:    listen,
				Handler: mux,
			}
			err := server.ListenAndServe()
			if err != nil {
				log.Error().Err(err).Msg("tunnel: failed to start server")
			}
		}
	}()
}
