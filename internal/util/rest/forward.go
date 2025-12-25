package rest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
)

var (
	forwardClient     *RESTClient
	forwardClientOnce sync.Once
)

// getForwardClient returns a shared REST client for forwarding requests
func getForwardClient() *RESTClient {
	forwardClientOnce.Do(func() {
		// Create client with insecureSkipVerify=true for internal cluster communication
		client, err := NewClient("http://localhost", "", true)
		if err != nil {
			log.Fatal("failed to create forward client: ", err)
		}
		client.SetTimeout(30 * time.Second)
		forwardClient = client
	})
	return forwardClient
}

// ForwardToNode forwards an HTTP request to another node in the cluster
func ForwardToNode(w http.ResponseWriter, r *http.Request, nodeId string) error {
	transport := service.GetTransport()

	// Get the node from gossip
	nodes := transport.Nodes()
	if nodes == nil {
		return errors.New("cluster not available")
	}

	var targetNode string
	for _, node := range nodes {
		if node.ID.String() == nodeId {
			targetNode = node.AdvertisedAddr()
			break
		}
	}

	if targetNode == "" {
		return errors.New("target node not found in cluster")
	}

	// Strip path from targetNode
	if idx := strings.Index(targetNode[8:], "/"); idx != -1 {
		targetNode = targetNode[:8+idx]
	}

	// Read request body
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body.Close()
	}

	// Create forwarded request using shared REST client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, r.Method, targetNode+r.URL.Path, bytes.NewReader(bodyBytes))
	if err != nil {
		return err
	}

	// Forward all headers from original request
	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Forward request using shared HTTP client
	client := getForwardClient()
	resp, err := client.HTTPClient.Do(req)
	if err != nil {
		log.WithError(err).Error("failed to forward request to node")
		return err
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	io.Copy(w, resp.Body)

	return nil
}

// ShouldForwardToNode checks if a request should be forwarded to another node
func ShouldForwardToNode(nodeId string) (bool, string) {
	if nodeId == "" {
		return false, ""
	}

	// Get local node ID from database
	db := database.GetInstance()
	nodeIdCfg, err := db.GetCfgValue("node_id")
	if err != nil || nodeIdCfg == nil {
		return false, ""
	}

	if nodeIdCfg.Value == nodeId {
		return false, ""
	}

	return true, nodeId
}
