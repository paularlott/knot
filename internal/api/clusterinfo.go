package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetClusterNode(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	clusterKey := r.Header.Get("X-Cluster-Key")
	if clusterKey != cfg.Cluster.Key {
		rest.WriteResponse(http.StatusUnauthorized, w, r, ErrorResponse{Error: "unauthorized"})
		return
	}

	db := database.GetInstance()
	nodeIdCfg, _ := db.GetCfgValue("node_id")
	localNodeId := ""
	if nodeIdCfg != nil {
		localNodeId = nodeIdCfg.Value
	}

	spaces, _ := db.GetSpaces()
	allocated, running := countSpaces(spaces, localNodeId)

	rest.WriteResponse(http.StatusOK, w, r, apiclient.ClusterNode{
		NodeId:          localNodeId,
		Hostname:        cfg.Hostname,
		Zone:            cfg.Zone,
		ApiEndpoint:     cfg.URL,
		AllocatedSpaces: allocated,
		RunningSpaces:   running,
		Runtimes:        runtime.DetectAllAvailableRuntimes(),
	})
}

func HandleGetClusterInfo(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	db := database.GetInstance()
	peers := service.GetTransport().Nodes()

	nodeIdCfg, _ := db.GetCfgValue("node_id")
	localNodeId := ""
	if nodeIdCfg != nil {
		localNodeId = nodeIdCfg.Value
	}

	spaces, _ := db.GetSpaces()
	response := make([]apiclient.ClusterNodeInfo, len(peers))

	for i, p := range peers {
		nodeId := p.ID.String()
		allocated, running := countSpaces(spaces, nodeId)

		var runtimes []string
		var hostname string
		if nodeId == localNodeId {
			runtimes = runtime.DetectAllAvailableRuntimes()
			hostname = cfg.Hostname
		} else {
			nodeInfo := queryNodeInfo(p.AdvertisedAddr(), cfg.Cluster.Key)
			runtimes = nodeInfo.Runtimes
			hostname = nodeInfo.Hostname
		}

		if hostname == "" {
			hostname = p.Metadata.GetString("hostname")
			if hostname == "" {
				hostname = "unknown"
			}
		}

		metadata := p.Metadata.GetAllAsString()
		metadata["hostname"] = hostname
		metadata["allocated_spaces"] = fmt.Sprintf("%d", allocated)
		metadata["running_spaces"] = fmt.Sprintf("%d", running)
		if len(runtimes) > 0 {
			metadata["runtimes"] = strings.Join(runtimes, ",")
		}

		response[i] = apiclient.ClusterNodeInfo{
			Id:       nodeId,
			Address:  p.AdvertisedAddr(),
			State:    p.GetObservedState().String(),
			Metadata: metadata,
		}
	}

	sort.Slice(response, func(i, j int) bool {
		return response[i].Address < response[j].Address
	})

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func queryNodeInfo(nodeAddr string, clusterKey string) apiclient.ClusterNode {
	client := &http.Client{Timeout: 2 * time.Second}
	
	// Strip path from nodeAddr (e.g., https://host/cluster -> https://host)
	if idx := strings.Index(nodeAddr[8:], "/"); idx != -1 {
		nodeAddr = nodeAddr[:8+idx]
	}
	
	req, err := http.NewRequest("GET", nodeAddr+"/api/cluster/node", nil)
	if err != nil {
		return apiclient.ClusterNode{}
	}
	req.Header.Set("X-Cluster-Key", clusterKey)

	resp, err := client.Do(req)
	if err != nil {
		return apiclient.ClusterNode{}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apiclient.ClusterNode{}
	}

	var nodeInfo apiclient.ClusterNode
	if err := json.NewDecoder(resp.Body).Decode(&nodeInfo); err != nil {
		return apiclient.ClusterNode{}
	}

	return nodeInfo
}

func countSpaces(spaces []*model.Space, nodeId string) (allocated int, running int) {
	for _, space := range spaces {
		if space.NodeId == nodeId && !space.IsDeleted {
			allocated++
			if space.IsDeployed {
				running++
			}
		}
	}
	return
}
