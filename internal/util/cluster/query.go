package cluster

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// QueryNodeRuntimes queries a remote node for its available runtimes
func QueryNodeRuntimes(nodeAddr string, clusterKey string) []string {
	client := &http.Client{Timeout: 2 * time.Second}
	
	// Strip path from nodeAddr
	if idx := strings.Index(nodeAddr[8:], "/"); idx != -1 {
		nodeAddr = nodeAddr[:8+idx]
	}
	
	req, err := http.NewRequest("GET", nodeAddr+"/api/cluster/node", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("X-Cluster-Key", clusterKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var nodeInfo struct {
		Runtimes []string `json:"runtimes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&nodeInfo); err != nil {
		return nil
	}

	return nodeInfo.Runtimes
}
