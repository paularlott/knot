package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/database"

	"github.com/paularlott/mcp"
)

type Group struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	MaxSpaces    uint32 `json:"max_spaces"`
	ComputeUnits uint32 `json:"compute_units"`
	StorageUnits uint32 `json:"storage_units"`
	MaxTunnels   uint32 `json:"max_tunnels"`
}

type GroupList struct {
	Groups []Group `json:"groups"`
}

func listGroups(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()
	groups, err := db.GetGroups()
	if err != nil {
		return nil, fmt.Errorf("Failed to get groups: %v", err)
	}

	var groupList GroupList
	for _, group := range groups {
		if group.IsDeleted {
			continue
		}

		groupList.Groups = append(groupList.Groups, Group{
			ID:           group.Id,
			Name:         group.Name,
			MaxSpaces:    group.MaxSpaces,
			ComputeUnits: group.ComputeUnits,
			StorageUnits: group.StorageUnits,
			MaxTunnels:   group.MaxTunnels,
		})
	}

	return mcp.NewToolResponseMulti(
		mcp.NewToolResponseJSON(groupList),
		mcp.NewToolResponseStructured(groupList),
	), nil
}
