package mcp

import (
	"context"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

func runCommand(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionRunCommands) {
		return nil, fmt.Errorf("No permission to run commands in spaces")
	}

	spaceId, err := req.String("space_id")
	if err != nil || spaceId == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_id is required")
	}

	command, err := req.String("command")
	if err != nil || command == "" {
		return nil, mcp.NewToolErrorInvalidParams("command is required")
	}

	timeout := req.IntOr("timeout", 30)
	workdir := req.StringOr("workdir", "")
	arguments := req.StringSliceOr("arguments", []string{})

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check permissions
	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to run commands in this space")
	}

	// Check if space is running
	if !space.IsDeployed {
		return nil, fmt.Errorf("Space is not running")
	}

	// Get the agent session
	session := agent_server.GetSession(spaceId)
	if session == nil {
		return nil, fmt.Errorf("Agent session not found for space")
	}

	// Send command to agent
	runCmd := &msg.RunCommandMessage{
		Command: command,
		Args:    arguments,
		Timeout: timeout,
		Workdir: workdir,
	}

	responseChannel, err := session.SendRunCommand(runCmd)
	if err != nil {
		return nil, fmt.Errorf("Failed to send command to agent: %v", err)
	}

	// Wait for response
	response := <-responseChannel
	if response == nil {
		return nil, fmt.Errorf("No response from agent")
	}

	result := map[string]interface{}{
		"space_id":   spaceId,
		"space_name": space.Name,
		"command":    command,
		"output":     string(response.Output),
		"success":    response.Success,
	}

	if !response.Success {
		result["error"] = response.Error
	}

	return mcp.NewToolResponseJSON(result), nil
}
