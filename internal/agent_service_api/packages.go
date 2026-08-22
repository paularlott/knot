package agent_service_api

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/agentpackages"
	"github.com/paularlott/knot/internal/log"
)

// fetchFromServer retrieves a path from the knot server with the agent's
// credentials — used to fill the package cache. knot.zip is a static server
// file; libs.zip is the authenticated per-user library package.
func fetchFromServer(path string) ([]byte, error) {
	server := agentClient.GetServerURL()
	url := server + path

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+agentClient.GetAgentToken())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for %s", resp.StatusCode, path)
	}
	return io.ReadAll(resp.Body)
}

// handlePackageKnotZip serves the knot.zip scriptling package from the agent
// cache, fetching it from the server on a miss. The cache is dropped whenever
// the agent establishes a new server session (see agentlink), so a server
// restart serves a fresh package.
func handlePackageKnotZip(w http.ResponseWriter, r *http.Request) {
	servePackage(w, r, "knot.zip", func() ([]byte, error) {
		return fetchFromServer("/packages/knot.zip")
	})
}

// handlePackageLibsZip serves the user's + global library scripts packaged by
// the server, from the agent cache. The server notifies the agent when a
// library changes, which drops the cache; the next request refetches.
func handlePackageLibsZip(w http.ResponseWriter, r *http.Request) {
	servePackage(w, r, "libs.zip", func() ([]byte, error) {
		return fetchFromServer("/api/scripts/libs.zip")
	})
}

func servePackage(w http.ResponseWriter, r *http.Request, name string, fetch func() ([]byte, error)) {
	data, err := agentpackages.Get(name, fetch)
	if err != nil {
		log.WithError(err).Warn("failed to fetch scriptling package", "package", name)
		http.Error(w, "package unavailable", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Write(data)
}

// handleConnect hands in-space processes the server URL and token via the
// local agent API — the HTTP equivalent of the agentlink socket connect. The
// token acts as the space owner; the port only listens inside the space.
func handleConnect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"server":%q,"token":%q}`, agentClient.GetServerURL(), agentClient.GetAgentToken())
}
