package api

import (
	"net/http"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/cluster"
	"github.com/paularlott/knot/internal/util/rest"
)

type AvailableNode struct {
	NodeId   string `json:"node_id"`
	Hostname string `json:"hostname"`
}

func HandleGetTemplateNodes(w http.ResponseWriter, r *http.Request) {
	templateId := r.PathValue("template_id")
	
	db := database.GetInstance()
	template, err := db.GetTemplate(templateId)
	if err != nil || template == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "template not found"})
		return
	}

	// Only return nodes for local container templates
	if !template.IsLocalContainer() {
		rest.WriteResponse(http.StatusOK, w, r, []AvailableNode{})
		return
	}

	cfg := config.GetServerConfig()
	transport := service.GetTransport()
	
	nodeIdCfg, _ := db.GetCfgValue("node_id")
	localNodeId := ""
	if nodeIdCfg != nil {
		localNodeId = nodeIdCfg.Value
	}

	var nodes []AvailableNode
	peers := transport.Nodes()

	if peers == nil {
		// Single server mode
		if hasRequiredRuntime(template, runtime.DetectAllAvailableRuntimes()) {
			nodes = append(nodes, AvailableNode{
				NodeId:   localNodeId,
				Hostname: cfg.Hostname,
			})
		}
	} else {
		// Cluster mode
		for _, peer := range peers {
			if peer.Metadata.GetString("zone") != cfg.Zone {
				continue
			}
			if peer.GetObservedState() != gossip.NodeAlive {
				continue
			}

			nodeId := peer.ID.String()
			var runtimes []string
			var hostname string
			
			if nodeId == localNodeId {
				runtimes = runtime.DetectAllAvailableRuntimes()
				hostname = cfg.Hostname
			} else {
				runtimes = cluster.QueryNodeRuntimes(peer.AdvertisedAddr(), cfg.Cluster.Key)
				hostname = peer.Metadata.GetString("hostname")
			}

			if hasRequiredRuntime(template, runtimes) {
				nodes = append(nodes, AvailableNode{
					NodeId:   nodeId,
					Hostname: hostname,
				})
			}
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, nodes)
}

func hasRequiredRuntime(template *model.Template, runtimes []string) bool {
	if template.Platform == model.PlatformContainer {
		return len(runtimes) > 0
	}

	for _, rt := range runtimes {
		if rt == template.Platform {
			return true
		}
	}
	return false
}
