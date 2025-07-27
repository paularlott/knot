package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

const (
	MCPProtocolVersionLatest = "2025-06-18"
	MCPProtocolVersionMin    = "2024-11-05"
)

var supportedProtocolVersions = []string{
	"2024-11-05",
	"2025-03-26",
	"2025-06-18",
}

// MCP Protocol types
type MCPRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type MCPResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *MCPError   `json:"error,omitempty"`
}

type MCPError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ClientInfo             `json:"clientInfo"`
}

type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type InitializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    Capabilities `json:"capabilities"`
	ServerInfo      ServerInfo   `json:"serverInfo"`
}

type Capabilities struct {
	Tools map[string]interface{} `json:"tools"`
}

type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Tool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

type ToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type ToolResult struct {
	Content []ToolContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type SpaceInfo struct {
	SpaceID     string `json:"space_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDeployed  bool   `json:"is_deployed"`
	IsPending   bool   `json:"is_pending"`
	IsDeleting  bool   `json:"is_deleting"`
	Zone        string `json:"zone"`
	UserID      string `json:"user_id"`
	Username    string `json:"username"`
}

func HandleMCP(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Max-Age", "86400")
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST, OPTIONS")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validate Content-Type
	contentType := r.Header.Get("Content-Type")
	if contentType != "application/json" && !strings.HasPrefix(contentType, "application/json;") {
		http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
		return
	}

	// Set CORS headers for actual requests
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendMCPError(w, nil, -32700, "Parse error", map[string]interface{}{
			"details": err.Error(),
		})
		return
	}

	// Validate JSONRPC version
	if req.JSONRPC != "2.0" {
		sendMCPError(w, req.ID, -32600, "Invalid Request", map[string]interface{}{
			"details": "JSONRPC field must be '2.0'",
		})
		return
	}

	// Ensure ID is never nil - use empty string as default
	if req.ID == nil {
		req.ID = ""
	}

	switch req.Method {
	case "initialize":
		handleInitialize(w, r, &req)
	case "tools/list":
		handleToolsList(w, r, &req)
	case "tools/call":
		handleToolsCall(w, r, &req)
	default:
		sendMCPError(w, req.ID, -32601, "Method not found", map[string]interface{}{
			"method": req.Method,
		})
	}
}

func isSupportedProtocolVersion(version string) bool {
	for _, supported := range supportedProtocolVersions {
		if supported == version {
			return true
		}
	}
	return false
}

func handleInitialize(w http.ResponseWriter, r *http.Request, req *MCPRequest) {
	// Parse initialization parameters
	var params InitializeParams
	if req.Params != nil {
		paramsBytes, err := json.Marshal(req.Params)
		if err != nil {
			sendMCPError(w, req.ID, -32602, "Invalid params", nil)
			return
		}
		if err := json.Unmarshal(paramsBytes, &params); err != nil {
			sendMCPError(w, req.ID, -32602, "Invalid params", nil)
			return
		}
	}

	// Determine which protocol version to use
	protocolVersion := MCPProtocolVersionLatest
	if params.ProtocolVersion != "" {
		if !isSupportedProtocolVersion(params.ProtocolVersion) {
			sendMCPError(w, req.ID, -32602, "Unsupported protocol version", map[string]interface{}{
				"requested": params.ProtocolVersion,
				"supported": supportedProtocolVersions,
			})
			return
		}
		protocolVersion = params.ProtocolVersion
	}

	result := InitializeResult{
		ProtocolVersion: protocolVersion,
		Capabilities:    buildCapabilities(protocolVersion),
		ServerInfo: ServerInfo{
			Name:    "knot-mcp-server",
			Version: "1.0.0",
		},
	}

	sendMCPResponse(w, req.ID, result)
}

func buildCapabilities(protocolVersion string) Capabilities {
	capabilities := Capabilities{
		Tools: map[string]interface{}{},
	}

	// Add version-specific capabilities
	switch protocolVersion {
	case "2024-11-05":
		// Basic capabilities for 2024-11-05
		capabilities.Tools = map[string]interface{}{}
	case "2025-03-26":
		// Enhanced capabilities for 2025-03-26
		capabilities.Tools = map[string]interface{}{
			"listChanged": false,
		}
	case "2025-06-18":
		// Latest capabilities for 2025-06-18
		capabilities.Tools = map[string]interface{}{
			"listChanged": false,
		}
	default:
		// Default to latest
		capabilities.Tools = map[string]interface{}{
			"listChanged": false,
		}
	}

	return capabilities
}

func handleToolsList(w http.ResponseWriter, r *http.Request, req *MCPRequest) {
	// You could store the negotiated protocol version in context during initialize
	// For now, we'll build tools that work across all supported versions

	tools := []Tool{
		{
			Name:        "list_spaces",
			Description: "List all spaces for a user or all users",
			InputSchema: buildToolSchema("list_spaces"),
		},
		{
			Name:        "start_space",
			Description: "Start a space by its ID",
			InputSchema: buildToolSchema("start_space"),
		},
		{
			Name:        "stop_space",
			Description: "Stop a space by its ID",
			InputSchema: buildToolSchema("stop_space"),
		},
	}

	result := map[string]interface{}{
		"tools": tools,
	}

	sendMCPResponse(w, req.ID, result)
}

func buildToolSchema(toolName string) map[string]interface{} {
	switch toolName {
	case "list_spaces":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"user_id": map[string]interface{}{
					"type":        "string",
					"description": "User ID to filter spaces (optional, empty for all users)",
				},
			},
			"additionalProperties": false,
		}
	case "start_space", "stop_space":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"space_id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the space to start/stop",
				},
			},
			"required":             []string{"space_id"},
			"additionalProperties": false,
		}
	default:
		return map[string]interface{}{
			"type": "object",
		}
	}
}

func handleToolsCall(w http.ResponseWriter, r *http.Request, req *MCPRequest) {
	// Get user from context (set by ApiAuth middleware)
	user := r.Context().Value("user").(*model.User)
	if user == nil {
		sendMCPError(w, req.ID, -32603, "Internal error: user not found in context", nil)
		return
	}

	var params ToolCallParams
	paramsBytes, err := json.Marshal(req.Params)
	if err != nil {
		sendMCPError(w, req.ID, -32602, "Invalid params", nil)
		return
	}

	if err := json.Unmarshal(paramsBytes, &params); err != nil {
		sendMCPError(w, req.ID, -32602, "Invalid params", nil)
		return
	}

	switch params.Name {
	case "list_spaces":
		handleListSpaces(w, r, req, user, params.Arguments)
	case "start_space":
		handleStartSpace(w, r, req, user, params.Arguments)
	case "stop_space":
		handleStopSpace(w, r, req, user, params.Arguments)
	default:
		sendMCPError(w, req.ID, -32601, "Tool not found", nil)
	}
}

func handleListSpaces(w http.ResponseWriter, r *http.Request, req *MCPRequest, user *model.User, args map[string]interface{}) {
	db := database.GetInstance()

	var userID string
	if uid, ok := args["user_id"].(string); ok {
		userID = uid
	}

	// If no user_id provided or user doesn't have manage permissions, use their own ID
	if userID == "" || (!user.HasPermission(model.PermissionManageSpaces) && !user.HasPermission(model.PermissionUseSpaces)) {
		userID = user.Id
	}

	spaces, err := db.GetSpacesForUser(userID)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Failed to get spaces: %v", err), nil)
		return
	}

	var spaceInfos []SpaceInfo
	for _, space := range spaces {
		spaceInfo := SpaceInfo{
			SpaceID:     space.Id,
			Name:        space.Name,
			Description: space.Description,
			IsDeployed:  space.IsDeployed,
			IsPending:   space.IsPending,
			IsDeleting:  space.IsDeleting,
			Zone:        space.Zone,
			UserID:      space.UserId,
		}

		// Get username
		if spaceUser, err := db.GetUser(space.UserId); err == nil {
			spaceInfo.Username = spaceUser.Username
		}

		spaceInfos = append(spaceInfos, spaceInfo)
	}

	result := ToolResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Found %d spaces:\n%s", len(spaceInfos), formatSpacesList(spaceInfos)),
			},
		},
	}

	sendMCPResponse(w, req.ID, result)
}

func handleStartSpace(w http.ResponseWriter, r *http.Request, req *MCPRequest, user *model.User, args map[string]interface{}) {
	spaceID, ok := args["space_id"].(string)
	if !ok || spaceID == "" {
		sendMCPError(w, req.ID, -32602, "space_id is required", nil)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Space not found: %v", err), nil)
		return
	}

	// Check if user has permission to start this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		sendMCPError(w, req.ID, -32603, "No permission to start this space", nil)
		return
	}

	// Get the templates
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Failed to get template: %v", err), nil)
		return
	}

	// Use the container service to start the space
	containerService := service.GetContainerService()
	err = containerService.StartSpace(space, template, user)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Failed to start space: %v", err), nil)
		return
	}

	result := ToolResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Space '%s' (%s) is starting", space.Name, spaceID),
			},
		},
	}

	sendMCPResponse(w, req.ID, result)
}

func handleStopSpace(w http.ResponseWriter, r *http.Request, req *MCPRequest, user *model.User, args map[string]interface{}) {
	spaceID, ok := args["space_id"].(string)
	if !ok || spaceID == "" {
		sendMCPError(w, req.ID, -32602, "space_id is required", nil)
		return
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceID)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Space not found: %v", err), nil)
		return
	}

	// Check if user has permission to stop this space
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		sendMCPError(w, req.ID, -32603, "No permission to stop this space", nil)
		return
	}

	// Use the container service to stop the space
	containerService := service.GetContainerService()
	err = containerService.StopSpace(space)
	if err != nil {
		sendMCPError(w, req.ID, -32603, fmt.Sprintf("Failed to stop space: %v", err), nil)
		return
	}

	result := ToolResult{
		Content: []ToolContent{
			{
				Type: "text",
				Text: fmt.Sprintf("Space '%s' (%s) is stopping", space.Name, spaceID),
			},
		},
	}

	sendMCPResponse(w, req.ID, result)
}

func formatSpacesList(spaces []SpaceInfo) string {
	if len(spaces) == 0 {
		return "No spaces found."
	}

	var builder strings.Builder
	for _, space := range spaces {
		status := "stopped"
		if space.IsDeleting {
			status = "deleting"
		} else if space.IsPending {
			status = "pending"
		} else if space.IsDeployed {
			status = "running"
		}

		builder.WriteString(fmt.Sprintf("- %s (%s): %s - %s\n",
			space.Name, space.SpaceID, status, space.Description))
	}

	return builder.String()
}

func sendMCPResponse(w http.ResponseWriter, id interface{}, result interface{}) {
	response := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

func sendMCPError(w http.ResponseWriter, id interface{}, code int, message string, data interface{}) {
	response := MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &MCPError{
			Code:    code,
			Message: message,
			Data:    data,
		},
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK) // Always 200 for JSON-RPC responses
	json.NewEncoder(w).Encode(response)
}
