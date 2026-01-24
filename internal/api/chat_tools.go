package api

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/mcp"
)

// HandleListTools handles GET /api/chat/tools - lists available tools
func HandleListTools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get MCP server from context
	mcpServer, ok := ctx.Value("mcp").(*mcp.Server)
	if !ok {
		log.Error("MCP server not found in context")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "MCP server not available",
		})
		return
	}

	// Get tools from MCP server with context to respect force ondemand mode
	tools := mcpServer.ListToolsWithContext(ctx)
	log.Debug("HandleListTools: returning tools", "count", len(tools))
	for i, tool := range tools {
		log.Debug("HandleListTools: tool", "index", i, "name", tool.Name, "description", tool.Description)
	}
	rest.WriteResponse(http.StatusOK, w, r, tools)
}

// HandleCallTool handles POST /api/chat/tools/call - calls a tool directly
func HandleCallTool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log.Debug("HandleCallTool: called", "path", r.URL.Path)

	// Check if user is in context (for debugging)
	if user, ok := ctx.Value("user").(*model.User); ok {
		log.Debug("HandleCallTool: user found in context", "username", user.Username)
	} else {
		log.Debug("HandleCallTool: user NOT found in context")
	}

	// Get MCP server from context
	mcpServer, ok := ctx.Value("mcp").(*mcp.Server)
	if !ok {
		log.Error("MCP server not found in context")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "MCP server not available",
		})
		return
	}
	log.Debug("HandleCallTool: mcp server found in context")

	// Parse request
	var req mcp.ToolCallParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.WithError(err).Error("Failed to decode tool call request")
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	// Validate request
	if req.Name == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Tool name is required",
		})
		return
	}

	if req.Arguments == nil {
		req.Arguments = make(map[string]interface{})
	}

	// Debug logging to see what's happening
	log.Debug("HandleCallTool: calling tool", "tool", req.Name, "user", r.Context().Value("user"))

	// Call the tool
	response, err := mcpServer.CallTool(ctx, req.Name, req.Arguments)

	// Log the response for debugging
	if err != nil {
		log.Debug("HandleCallTool: tool call failed", "tool", req.Name, "error", err)
	} else {
		log.Debug("HandleCallTool: tool call succeeded", "tool", req.Name, "has_content", response != nil)
	}
	if err != nil {
		log.WithError(err).Error("Tool call failed", "tool", req.Name)
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Tool call failed: " + err.Error(),
		})
		return
	}

	// Return the result - *mcp.ToolResponse marshals correctly as JSON
	rest.WriteResponse(http.StatusOK, w, r, response)
}
