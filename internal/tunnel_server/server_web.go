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
	"github.com/paularlott/knot/internal/log"
)

// TunnelRequestLogHook is called after each request proxied through a web
// tunnel, carrying the tunnel owner and the request outcome. The pro build
// assigns it (system-log forwarding + log-sink mirroring); the OSS build
// leaves it nil and tunnel requests are unlogged.
var TunnelRequestLogHook func(userId, username, tunnelName, method, path, host string, status int, duration time.Duration)

// TunnelLifecycleLogHook is called when a tunnel opens or closes, so the
// pro build can mirror the lifecycle into the owner's log sinks (the audit
// events cover the audit trail; sinks only ever see log records). The OSS
// build leaves it nil.
var TunnelLifecycleLogHook func(userId, username, tunnelLabel string, opened bool)

// HandleWebTunnel handles web tunnel requests for domain-based routing
func HandleWebTunnel(w http.ResponseWriter, r *http.Request) {
	logger := log.WithGroup("tunnel")
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
		logger.Error("not found", "host", r.Host, "path", r.URL.Path, "domainParts0", domainParts[0])
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
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	start := time.Now()
	httpProxy.ServeHTTP(rec, r)
	if TunnelRequestLogHook != nil {
		TunnelRequestLogHook(session.user.Id, session.user.Username, session.tunnelName,
			r.Method, r.URL.Path, r.Host, rec.status, time.Since(start))
	}
}

// statusRecorder captures the response status for request logging while
// delegating everything else to the wrapped writer.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Start a web server to listen for connections to tunnels, the left most part of the domain is the <username>--<tunnel name>
func ListenAndServe(listen string, tlsConfig *tls.Config) {
	logger := log.WithGroup("tunnel")
	logger.Info("listening on", "listen", listen)

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
				logger.WithError(err).Error("failed to start server")
			}
		} else {
			server := &http.Server{
				Addr:    listen,
				Handler: mux,
			}
			err := server.ListenAndServe()
			if err != nil {
				logger.WithError(err).Error("failed to start server")
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
