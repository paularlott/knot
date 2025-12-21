package cluster

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

// ClusterNodeInfo represents basic node information returned from queries
type ClusterNodeInfo struct {
	NodeId          string   `json:"node_id"`
	Hostname        string   `json:"hostname"`
	Zone            string   `json:"zone"`
	ApiEndpoint     string   `json:"api_endpoint"`
	AllocatedSpaces int      `json:"allocated_spaces"`
	RunningSpaces   int      `json:"running_spaces"`
	Runtimes        []string `json:"runtimes"`
}

// QueryNodeInfo queries a remote node for its information including runtimes
func QueryNodeInfo(nodeAddr string, clusterKey string) ClusterNodeInfo {
	client := &http.Client{Timeout: 2 * time.Second}

	// Strip path from nodeAddr (e.g., https://host/cluster -> https://host)
	if idx := strings.Index(nodeAddr[8:], "/"); idx != -1 {
		nodeAddr = nodeAddr[:8+idx]
	}

	req, err := http.NewRequest("GET", nodeAddr+"/api/cluster/node", nil)
	if err != nil {
		return ClusterNodeInfo{}
	}
	req.Header.Set("X-Cluster-Key", clusterKey)

	resp, err := client.Do(req)
	if err != nil {
		return ClusterNodeInfo{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ClusterNodeInfo{}
	}

	var nodeInfo ClusterNodeInfo
	if err := json.NewDecoder(resp.Body).Decode(&nodeInfo); err != nil {
		return ClusterNodeInfo{}
	}

	return nodeInfo
}

// QueryNodeRuntimes queries a remote node for its available runtimes
// This is a convenience wrapper around QueryNodeInfo
func QueryNodeRuntimes(nodeAddr string, clusterKey string) []string {
	return QueryNodeInfo(nodeAddr, clusterKey).Runtimes
}
