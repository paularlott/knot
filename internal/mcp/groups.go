package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/validate"

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

func listGroups(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	db := database.GetInstance()
	groups, err := db.GetGroups()
	if err != nil {
		return nil, fmt.Errorf("Failed to get groups: %v", err)
	}

	var result []Group
	for _, group := range groups {
		if group.IsDeleted {
			continue
		}

		result = append(result, Group{
			ID:           group.Id,
			Name:         group.Name,
			MaxSpaces:    group.MaxSpaces,
			ComputeUnits: group.ComputeUnits,
			StorageUnits: group.StorageUnits,
			MaxTunnels:   group.MaxTunnels,
		})
	}

	return mcp.NewToolResponseJSON(result), nil
}

func createGroup(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageGroups) {
		return nil, fmt.Errorf("No permission to manage groups")
	}

	name := req.StringOr("name", "")
	if !validate.Required(name) || !validate.MaxLength(name, 64) {
		return nil, fmt.Errorf("Invalid user group name")
	}

	maxSpaces := req.IntOr("max_spaces", 0)
	if !validate.IsNumber(maxSpaces, 0, 10000) {
		return nil, fmt.Errorf("Invalid max spaces")
	}

	computeUnits := req.IntOr("compute_units", 0)
	if !validate.IsPositiveNumber(computeUnits) {
		return nil, fmt.Errorf("Invalid compute units")
	}

	storageUnits := req.IntOr("storage_units", 0)
	if !validate.IsPositiveNumber(storageUnits) {
		return nil, fmt.Errorf("Invalid storage units")
	}

	maxTunnels := req.IntOr("max_tunnels", 0)
	if !validate.IsPositiveNumber(maxTunnels) {
		return nil, fmt.Errorf("Invalid tunnel limit")
	}

	group := model.NewGroup(name, user.Id, uint32(maxSpaces), uint32(computeUnits), uint32(storageUnits), uint32(maxTunnels))
	err := database.GetInstance().SaveGroup(group)
	if err != nil {
		return nil, fmt.Errorf("Failed to save group: %v", err)
	}

	service.GetTransport().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupCreate,
		fmt.Sprintf("Created group %s", group.Name),
		&map[string]interface{}{
			"group_id":   group.Id,
			"group_name": group.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
		"id":     group.Id,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func updateGroup(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageGroups) {
		return nil, fmt.Errorf("No permission to manage groups")
	}

	groupId := req.StringOr("group_id", "")
	if !validate.UUID(groupId) {
		return nil, fmt.Errorf("Invalid group ID")
	}

	db := database.GetInstance()
	group, err := db.GetGroup(groupId)
	if err != nil {
		return nil, fmt.Errorf("Group not found: %v", err)
	}

	// Only update name if provided
	if name, err := req.String("name"); err != mcp.ErrUnknownParameter {
		if !validate.Required(name) || !validate.MaxLength(name, 64) {
			return nil, fmt.Errorf("Invalid user group name")
		}
		group.Name = name
	}

	// Only update max_spaces if provided
	if maxSpaces, err := req.Int("max_spaces"); err != mcp.ErrUnknownParameter {
		if !validate.IsNumber(maxSpaces, 0, 10000) {
			return nil, fmt.Errorf("Invalid max spaces")
		}
		group.MaxSpaces = uint32(maxSpaces)
	}

	// Only update compute_units if provided
	if computeUnits, err := req.Int("compute_units"); err != mcp.ErrUnknownParameter {
		if !validate.IsPositiveNumber(computeUnits) {
			return nil, fmt.Errorf("Invalid compute units")
		}
		group.ComputeUnits = uint32(computeUnits)
	}

	// Only update storage_units if provided
	if storageUnits, err := req.Int("storage_units"); err != mcp.ErrUnknownParameter {
		if !validate.IsPositiveNumber(storageUnits) {
			return nil, fmt.Errorf("Invalid storage units")
		}
		group.StorageUnits = uint32(storageUnits)
	}

	// Only update max_tunnels if provided
	if maxTunnels, err := req.Int("max_tunnels"); err != mcp.ErrUnknownParameter {
		if !validate.IsPositiveNumber(maxTunnels) {
			return nil, fmt.Errorf("Invalid tunnel limit")
		}
		group.MaxTunnels = uint32(maxTunnels)
	}

	group.UpdatedAt = hlc.Now()
	group.UpdatedUserId = user.Id

	err = db.SaveGroup(group)
	if err != nil {
		return nil, fmt.Errorf("Failed to save group: %v", err)
	}

	service.GetTransport().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupUpdate,
		fmt.Sprintf("Updated group %s", group.Name),
		&map[string]interface{}{
			"group_id":   group.Id,
			"group_name": group.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}

func deleteGroup(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionManageGroups) {
		return nil, fmt.Errorf("No permission to manage groups")
	}

	groupId := req.StringOr("group_id", "")
	if !validate.UUID(groupId) {
		return nil, fmt.Errorf("Invalid group ID")
	}

	db := database.GetInstance()
	group, err := db.GetGroup(groupId)
	if err != nil {
		return nil, fmt.Errorf("Group not found: %v", err)
	}

	group.IsDeleted = true
	group.UpdatedAt = hlc.Now()
	group.UpdatedUserId = user.Id
	err = db.SaveGroup(group)
	if err != nil {
		return nil, fmt.Errorf("Failed to delete group: %v", err)
	}

	service.GetTransport().GossipGroup(group)

	audit.Log(
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventGroupDelete,
		fmt.Sprintf("Deleted group %s", group.Name),
		&map[string]interface{}{
			"group_id":   group.Id,
			"group_name": group.Name,
		},
	)

	result := map[string]interface{}{
		"status": true,
	}

	return mcp.NewToolResponseJSON(result), nil
}
