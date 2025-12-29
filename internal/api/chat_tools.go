package api

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/middleware"
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

	// Get tools from MCP server and return them directly
	tools := mcpServer.ListTools()
	rest.WriteResponse(http.StatusOK, w, r, tools)
}

// HandleCallTool handles POST /api/chat/tools/call - calls a tool directly
func HandleCallTool(w http.ResponseWriter, r *http.Request) {
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

	// Call the tool
	response, err := mcpServer.CallTool(ctx, req.Name, req.Arguments)
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

// RegisterChatToolRoutes registers the chat tool API routes
func RegisterChatToolRoutes(router *http.ServeMux) {
	router.HandleFunc("GET /api/chat/tools", middleware.ApiAuth(middleware.ApiPermissionUseWebAssistant(HandleListTools)))
	router.HandleFunc("POST /api/chat/tools/call", middleware.ApiAuth(middleware.ApiPermissionUseWebAssistant(HandleCallTool)))
}