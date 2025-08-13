package tunnel_server

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/rs/zerolog/log"
)

// HandleWebTunnel handles web tunnel requests for domain-based routing
func HandleWebTunnel(w http.ResponseWriter, r *http.Request) {
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
	if !ok || session.tunnelType != WebTunnel {
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

	httpProxy := reverseProxy(targetURL, stream, nil, r.Host)
	httpProxy.ServeHTTP(w, r)
}

// Start a web server to listen for connections to tunnels, the left most part of the domain is the <username>--<tunnel name>
func ListenAndServe(listen string, tlsConfig *tls.Config) {
	log.Info().Msgf("tunnel: listening on %s", listen)

	go func() {
		mux := http.NewServeMux()
		mux.HandleFunc("/", HandleWebTunnel)

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

func reverseProxy(targetURL *url.URL, stream net.Conn, accessToken *string, host string) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Proxy", "knot "+build.Version)
		req.URL.Scheme = "http" // Force http as the agent will upgrade the connection to https

		// Set the host header
		if host != "" {
			req.Host = host
		} else {
			req.Host = targetURL.Host // Set the Host header
		}

		if accessToken != nil {
			req.Header.Set("Authorization", *accessToken)
		}
	}

	proxy.Transport = &http.Transport{
		TLSClientConfig:     &tls.Config{InsecureSkipVerify: true},
		MaxConnsPerHost:     32 * 2,
		MaxIdleConns:        32 * 2,
		MaxIdleConnsPerHost: 32,
		IdleConnTimeout:     30 * time.Second,
		DisableCompression:  false,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return stream, nil
		},
	}

	return proxy
}
