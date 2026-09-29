package api

import (
	"encoding/json"
	"net/http"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util/rest"

	"github.com/paularlott/mcp"
)

// These two endpoints are scriptling's knot.mcp transport (list_tools /
// call_tool / tool_search / execute_tool in internal/scriptling/mcp_library.go,
// which rides them via the in-process mux client or a remote API client).
// They resolve tools exactly like the web chat does: the internal MCP
// server plus the per-user script/method/remote-server providers that
// MCPServerContext injects, so a script sees the same tools (including its
// user's remote MCP servers) as the chat. They are not part of the public
// /mcp endpoint, which serves knot's own tools only.

// HandleListTools handles GET /api/chat/tools — lists available tools.
func HandleListTools(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mcpServer, ok := ctx.Value("mcp").(*mcp.Server)
	if !ok {
		log.Error("MCP server not found in context")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "MCP server not available",
		})
		return
	}

	tools := mcpServer.ListToolsWithContext(ctx)
	rest.WriteResponse(http.StatusOK, w, r, tools)
}

// HandleCallTool handles POST /api/chat/tools/call — calls a tool directly.
// The tool_search and execute_tool virtual tools resolve through the same
// call, exactly as they do for the chat.
func HandleCallTool(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mcpServer, ok := ctx.Value("mcp").(*mcp.Server)
	if !ok {
		log.Error("MCP server not found in context")
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "MCP server not available",
		})
		return
	}

	var req mcp.ToolCallParams
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Invalid request body",
		})
		return
	}
	if req.Name == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, map[string]string{
			"error": "Tool name is required",
		})
		return
	}
	if req.Arguments == nil {
		req.Arguments = make(map[string]interface{})
	}

	response, err := mcpServer.CallTool(ctx, req.Name, req.Arguments)
	if err != nil {
		log.WithError(err).Error("Tool call failed", "tool", req.Name)
		rest.WriteResponse(http.StatusInternalServerError, w, r, map[string]string{
			"error": "Tool call failed: " + err.Error(),
		})
		return
	}

	rest.WriteResponse(http.StatusOK, w, r, response)
}
