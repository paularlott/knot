package service

import (
	"errors"
	"math/rand"
	"strings"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container/runtime"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
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

	// Only node-local runtimes (containers and KVM) are pinned to a node
	if !template.IsNodeRuntime() {
		return "", nil
	}

	cfg := config.GetServerConfig()
	db := database.GetInstance()
	transport := GetTransport()

	// Get local node ID
	localNodeId := localNodeId()
	if localNodeId == "" {
		return "", errors.New("failed to get local node ID")
	}

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
		if hasRequiredRuntime(template, runtime.DetectAllAvailableRuntimesWithKVM()) {
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
				runtimes = runtime.DetectAllAvailableRuntimesWithKVM()
			} else {
				runtimes = strings.Split(peer.Metadata.GetString("runtimes"), ",")
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

// AvailableZoneRuntimes returns the set of runtimes offered by alive nodes
// in this zone — the local node's (detected once, 30s-cached) plus gossip
// peers' advertised runtimes. List endpoints build this once and use
// TemplateRuntimeAvailableIn for per-template checks, so a page of templates
// costs one detection burst rather than one per row.
func AvailableZoneRuntimes() map[string]bool {
	cfg := config.GetServerConfig()
	available := map[string]bool{}

	add := func(list []string) {
		for _, rt := range list {
			if rt = strings.TrimSpace(rt); rt != "" {
				available[rt] = true
			}
		}
	}

	if peers := GetTransport().Nodes(); peers != nil {
		local := localNodeId()
		for _, peer := range peers {
			if peer.Metadata.GetString("zone") != cfg.Zone {
				continue
			}
			if peer.GetObservedState() != gossip.NodeAlive {
				continue
			}
			if peer.ID.String() == local {
				add(runtime.DetectAllAvailableRuntimesWithKVM())
			} else {
				add(strings.Split(peer.Metadata.GetString("runtimes"), ","))
			}
		}
		return available
	}

	add(runtime.DetectAllAvailableRuntimesWithKVM())
	return available
}

// TemplateRuntimeAvailableIn is the pure membership check against a set
// built by AvailableZoneRuntimes — the same semantics SelectNodeForSpace
// applies. Manual and Nomad templates are always available (no knot-managed
// runtime involved); the "container" platform matches any container runtime
// but not KVM alone.
func TemplateRuntimeAvailableIn(template *model.Template, available map[string]bool) bool {
	if template.Platform == model.PlatformNomad || template.Platform == model.PlatformManual {
		return true
	}
	if !template.IsNodeRuntime() {
		return true
	}
	if template.Platform == model.PlatformContainer {
		for rt := range available {
			if rt != model.PlatformKvm {
				return true
			}
		}
		return false
	}
	return available[template.Platform]
}

// TemplateRuntimeAvailable reports whether any node in the zone currently
// offers the runtime a template needs. Builds the zone set on each call —
// fine for one-off checks; list endpoints should use AvailableZoneRuntimes
// plus TemplateRuntimeAvailableIn instead.
func TemplateRuntimeAvailable(template *model.Template) bool {
	return TemplateRuntimeAvailableIn(template, AvailableZoneRuntimes())
}

func localNodeId() string {
	nodeIdCfg, err := database.GetInstance().GetCfgValue("node_id")
	if err != nil || nodeIdCfg == nil {
		return ""
	}
	return nodeIdCfg.Value
}

func hasRequiredRuntime(template *model.Template, runtimes []string) bool {
	if template.Platform == model.PlatformContainer {
		// "container" means any local *container* runtime — KVM alone does
		// not satisfy it.
		for _, rt := range runtimes {
			if rt != model.PlatformKvm {
				return true
			}
		}
		return false
	}

	for _, rt := range runtimes {
		if rt == template.Platform {
			return true
		}
	}
	return false
}
