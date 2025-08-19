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

func copyFile(ctx context.Context, req *mcp.ToolRequest) (*mcp.ToolResponse, error) {
	user := ctx.Value("user").(*model.User)
	if !user.HasPermission(model.PermissionCopyFiles) {
		return nil, fmt.Errorf("No permission to copy files in spaces")
	}

	spaceName, err := req.String("space_name")
	if err != nil || spaceName == "" {
		return nil, mcp.NewToolErrorInvalidParams("space_name is required")
	}

	spaceId, err := resolveSpaceNameToID(spaceName, user)
	if err != nil {
		return nil, err
	}

	// Check if content is provided (write to space) or path is provided (read from space)
	content := req.StringOr("content", "")
	sourcePath := req.StringOr("source_path", "")
	destPath := req.StringOr("dest_path", "")

	var direction string
	var copyCmd *msg.CopyFileMessage

	if content != "" && destPath != "" {
		// Write content to space - content can be base64 encoded or plain text
		direction = "to_space"
		var contentBytes []byte

		// Try to decode as base64 first, if that fails treat as plain text
		if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
			contentBytes = decoded
		} else {
			contentBytes = []byte(content)
		}

		copyCmd = &msg.CopyFileMessage{
			DestPath:  destPath,
			Content:   contentBytes,
			Direction: direction,
			Workdir:   "",
		}
	} else if sourcePath != "" {
		// Read from space
		direction = "from_space"
		copyCmd = &msg.CopyFileMessage{
			SourcePath: sourcePath,
			Direction:  direction,
			Workdir:    "",
		}
	} else {
		return nil, mcp.NewToolErrorInvalidParams("Either (content and dest_path) or source_path must be provided")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check permissions
	if space.UserId != user.Id && space.SharedWithUserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to copy files in this space")
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

	// Send copy file command to agent
	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		return nil, fmt.Errorf("Failed to send copy file command to agent: %v", err)
	}

	// Wait for response
	response := <-responseChannel
	if response == nil {
		return nil, fmt.Errorf("No response from agent")
	}

	result := map[string]interface{}{
		"space_id":   spaceId,
		"space_name": space.Name,
		"direction":  direction,
		"success":    response.Success,
	}

	if !response.Success {
		result["error"] = response.Error
		return mcp.NewToolResponseJSON(result), nil
	}

	if direction == "to_space" {
		result["message"] = fmt.Sprintf("Successfully wrote %d bytes to %s", len(copyCmd.Content), destPath)
		result["dest_path"] = destPath
	} else {
		result["content"] = string(response.Content)
		result["source_path"] = sourcePath
		result["size"] = len(response.Content)
	}

	return mcp.NewToolResponseJSON(result), nil
}
