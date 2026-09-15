package api

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

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
	response := make([]apiclient.ClusterNodeInfo, 0, len(peers)+1)

	selfSeen := false
	for _, p := range peers {
		nodeId := p.ID.String()
		if nodeId == localNodeId {
			selfSeen = true
		}
		allocated, running := countSpaces(spaces, nodeId)

		hostname := cfg.Hostname
		if nodeId != localNodeId {
			hostname = p.Metadata.GetString("hostname")
		}
		if hostname == "" {
			hostname = "unknown"
		}

		metadata := p.Metadata.GetAllAsString()
		metadata["hostname"] = hostname
		metadata["allocated_spaces"] = fmt.Sprintf("%d", allocated)
		metadata["running_spaces"] = fmt.Sprintf("%d", running)
		if nodeId == localNodeId {
			if runtimes := runtime.DetectAllAvailableRuntimes(); len(runtimes) > 0 {
				metadata["runtimes"] = strings.Join(runtimes, ",")
			}
		}

		response = append(response, apiclient.ClusterNodeInfo{
			Id:       nodeId,
			Address:  p.AdvertisedAddr(),
			State:    p.GetObservedState().String(),
			Metadata: metadata,
		})
	}

	// A standalone server (or a node whose gossip membership hasn't synced)
	// never appears in the peer list; report the local node so cluster info
	// is complete on single-server deployments.
	if !selfSeen && localNodeId != "" {
		allocated, running := countSpaces(spaces, localNodeId)
		metadata := map[string]string{
			"zone":             cfg.Zone,
			"hostname":         cfg.Hostname,
			"allocated_spaces": fmt.Sprintf("%d", allocated),
			"running_spaces":   fmt.Sprintf("%d", running),
		}
		if runtimes := runtime.DetectAllAvailableRuntimes(); len(runtimes) > 0 {
			metadata["runtimes"] = strings.Join(runtimes, ",")
		}
		response = append(response, apiclient.ClusterNodeInfo{
			Id:       localNodeId,
			Address:  cfg.Cluster.AdvertiseAddr,
			State:    "alive",
			Metadata: metadata,
		})
	}

	sort.Slice(response, func(i, j int) bool {
		return response[i].Address < response[j].Address
	})

	rest.WriteResponse(http.StatusOK, w, r, response)
}

func countSpaces(spaces []*model.Space, nodeId string) (allocated int, running int) {
	for _, space := range spaces {
		if space.IsDeleted {
			continue
		}
		// Exact node assignment only. A space with no node assignment
		// (manual and Nomad platforms, or spaces predating node pinning)
		// belongs to no specific node — counting it here would attribute it
		// to every node in the zone.
		if space.NodeId == nodeId {
			allocated++
			if space.IsDeployed {
				running++
			}
		}
	}
	return
}
