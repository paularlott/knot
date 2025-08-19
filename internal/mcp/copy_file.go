package mcp

import (
	"context"
	"encoding/base64"
	"fmt"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"

	"github.com/paularlott/mcp"
)

func readFile(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionCopyFiles) {
		return nil, fmt.Errorf("No permission to read files in spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	filePath, err := req.String("file_path")
	if err != nil || filePath == "" {
		return nil, mcp.NewToolErrorInvalidParams("file_path is required")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to read files in this space")
	}

	if !space.IsDeployed {
		return nil, fmt.Errorf("Space is not running")
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		return nil, fmt.Errorf("Agent session not found for space")
	}

	copyCmd := &msg.CopyFileMessage{
		SourcePath: filePath,
		Direction:  "from_space",
		Workdir:    "",
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		return nil, fmt.Errorf("Failed to send read file command to agent: %v", err)
	}

	response := <-responseChannel
	if response == nil {
		return nil, fmt.Errorf("No response from agent")
	}

	result := map[string]interface{}{
		"space_name": space.Name,
		"file_path":  filePath,
		"success":    response.Success,
	}

	if !response.Success {
		result["error"] = response.Error
	} else {
		result["content"] = string(response.Content)
		result["size"] = len(response.Content)
	}

	return mcp.NewToolResponseJSON(result), nil
}

func writeFile(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionCopyFiles) {
		return nil, fmt.Errorf("No permission to write files in spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	filePath, err := req.String("file_path")
	if err != nil || filePath == "" {
		return nil, mcp.NewToolErrorInvalidParams("file_path is required")
	}

	content, err := req.String("content")
	if err != nil {
		return nil, mcp.NewToolErrorInvalidParams("content is required")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to write files in this space")
	}

	if !space.IsDeployed {
		return nil, fmt.Errorf("Space is not running")
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		return nil, fmt.Errorf("Agent session not found for space")
	}

	var contentBytes []byte
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
		contentBytes = decoded
	} else {
		contentBytes = []byte(content)
	}

	copyCmd := &msg.CopyFileMessage{
		DestPath:  filePath,
		Content:   contentBytes,
		Direction: "to_space",
		Workdir:   "",
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		return nil, fmt.Errorf("Failed to send write file command to agent: %v", err)
	}

	response := <-responseChannel
	if response == nil {
		return nil, fmt.Errorf("No response from agent")
	}

	result := map[string]interface{}{
		"space_name": space.Name,
		"file_path":  filePath,
		"success":    response.Success,
	}

	if !response.Success {
		result["error"] = response.Error
	} else {
		result["message"] = fmt.Sprintf("Successfully wrote %d bytes to %s", len(contentBytes), filePath)
		result["bytes_written"] = len(contentBytes)
	}

	return mcp.NewToolResponseJSON(result), nil
}
