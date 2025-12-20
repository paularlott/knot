package service

import (
	"errors"
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/cluster"
)

type nodeCandidate struct {
	nodeId          string
	allocatedSpaces int
	runningSpaces   int
}

// SelectNodeForSpace selects the best node for a space based on template requirements
// Returns node ID or empty string for auto-selection, or error if no suitable node found
func SelectNodeForSpace(template *model.Template, selectedNodeId string) (string, error) {
	// Skip node selection for nomad and manual platforms
	if template.Platform == model.PlatformNomad || template.Platform == model.PlatformManual {
		return "", nil
	}

	// If not a local container template, skip node selection
	if !template.IsLocalContainer() {
		return "", nil
	}

	cfg := config.GetServerConfig()
	db := database.GetInstance()
	transport := GetTransport()

	// Get local node ID
	nodeIdCfg, err := db.GetCfgValue("node_id")
	if err != nil || nodeIdCfg == nil {
		return "", errors.New("failed to get local node ID")
	}
	localNodeId := nodeIdCfg.Value

	// Get all spaces for counting
	spaces, err := db.GetSpaces()
	if err != nil {
		return "", err
	}

	// Build map of space counts per node
	spaceCounts := make(map[string]*nodeCandidate)
	for _, space := range spaces {
		if space.NodeId != "" && !space.IsDeleted {
			if _, exists := spaceCounts[space.NodeId]; !exists {
				spaceCounts[space.NodeId] = &nodeCandidate{nodeId: space.NodeId}
			}
			spaceCounts[space.NodeId].allocatedSpaces++
			if space.IsDeployed {
				spaceCounts[space.NodeId].runningSpaces++
			}
		}
	}

	// Get eligible nodes
	var candidates []*nodeCandidate
	peers := transport.Nodes()

	if peers == nil {
		// Single server mode - check if local node has required runtime
		if hasRequiredRuntime(template, runtime.DetectAllAvailableRuntimes()) {
			candidate := spaceCounts[localNodeId]
			if candidate == nil {
				candidate = &nodeCandidate{nodeId: localNodeId}
			}
			candidates = append(candidates, candidate)
		}
	} else {
		// Cluster mode - check all nodes in zone
		for _, peer := range peers {
			if peer.Metadata.GetString("zone") != cfg.Zone {
				continue
			}
			// Only consider alive nodes
			if peer.GetObservedState() != gossip.NodeAlive {
				continue
			}

			nodeId := peer.ID.String()
			var runtimes []string
			if nodeId == localNodeId {
				runtimes = runtime.DetectAllAvailableRuntimes()
			} else {
				// Query remote node for runtimes
				runtimes = cluster.QueryNodeRuntimes(peer.AdvertisedAddr(), cfg.Cluster.Key)
			}

			if hasRequiredRuntime(template, runtimes) {
				candidate := spaceCounts[nodeId]
				if candidate == nil {
					candidate = &nodeCandidate{nodeId: nodeId}
				}
				candidates = append(candidates, candidate)
			}
		}
	}

	if len(candidates) == 0 {
		return "", errors.New("no nodes available with required runtime")
	}

	// If user selected a specific node, validate and use it
	if selectedNodeId != "" {
		for _, c := range candidates {
			if c.nodeId == selectedNodeId {
				return selectedNodeId, nil
			}
		}
		return "", errors.New("selected node not available or does not support required runtime")
	}

	// Auto-select: find node with lowest allocated spaces
	bestCandidate := candidates[0]
	for _, c := range candidates[1:] {
		if c.allocatedSpaces < bestCandidate.allocatedSpaces {
			bestCandidate = c
		} else if c.allocatedSpaces == bestCandidate.allocatedSpaces {
			// Tie-breaker: lowest running spaces
			if c.runningSpaces < bestCandidate.runningSpaces {
				bestCandidate = c
			} else if c.runningSpaces == bestCandidate.runningSpaces {
				// Random selection on tie
				if rand.Intn(2) == 0 {
					bestCandidate = c
				}
			}
		}
	}

	return bestCandidate.nodeId, nil
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
