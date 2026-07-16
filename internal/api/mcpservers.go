package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	internalmcp "github.com/paularlott/knot/internal/mcp"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

func HandleGetMCPServers(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	filterUserId := user.Id

	// Users with manage permission can view other users' servers
	cfg := config.GetServerConfig()
	if !cfg.LeafNode && user.HasPermission(model.PermissionManageMCPServers) {
		if q := r.URL.Query().Get("user_id"); q != "" {
			filterUserId = q
		}
	}

	db := database.GetInstance()
	servers, err := db.GetMCPServers()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	var result []apiclient.MCPServerInfo
	for _, s := range servers {
		if s.IsDeleted {
			continue
		}
		if filterUserId != "" && s.UserId != filterUserId {
			continue
		}
		result = append(result, toMCPServerInfo(s))
	}

	if result == nil {
		result = []apiclient.MCPServerInfo{}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Namespace < result[j].Namespace
	})

	rest.WriteResponse(http.StatusOK, w, r, &apiclient.MCPServerList{Count: len(result), Servers: result})
}

func HandleGetMCPServer(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	serverId := r.PathValue("mcp_server_id")

	if !validate.UUID(serverId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid MCP server ID"})
		return
	}

	db := database.GetInstance()
	server, err := db.GetMCPServer(serverId)
	if err != nil || server.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "MCP server not found"})
		return
	}

	cfg := config.GetServerConfig()
	if !cfg.LeafNode {
		if server.UserId != user.Id && !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to view this MCP server"})
			return
		}
	}

	rest.WriteResponse(http.StatusOK, w, r, toMCPServerDetails(server))
}

func HandleCreateMCPServer(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)

	request := apiclient.MCPServerCreateRequest{}
	err := rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	if request.Namespace == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Namespace is required"})
		return
	}

	if request.Command == "" && request.URL == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Either URL or Command must be set"})
		return
	}

	// MCP servers are always per-user — assign to the current user.
	ownerUserId := user.Id

	cfg := config.GetServerConfig()
	if !cfg.LeafNode && !user.HasPermission(model.PermissionManageMCPServers) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to create MCP servers"})
		return
	}

	// Check namespace uniqueness per user
	db := database.GetInstance()
	existing, _ := db.GetMCPServers()
	for _, s := range existing {
		if !s.IsDeleted && s.UserId == ownerUserId && s.Namespace == request.Namespace {
			rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: fmt.Sprintf("MCP server with namespace %q already exists", request.Namespace)})
			return
		}
	}

	server := model.NewMCPServer(request.Namespace, ownerUserId, user.Id)
	server.URL = strings.TrimSuffix(request.URL, "/")
	server.Command = request.Command
	server.Args = request.Args
	server.Env = request.Env
	server.AuthType = request.AuthType
	server.Token = request.Token
	server.OAuthClientID = request.OAuthClientID
	server.OAuthTokenURL = request.OAuthTokenURL
	server.OAuthAccessToken = request.OAuthAccessToken
	server.OAuthRefreshToken = request.OAuthRefreshToken
	server.Enabled = request.Enabled
	server.RemoteSearch = request.RemoteSearch
	if request.ToolVisibility != "" {
		server.ToolVisibility = request.ToolVisibility
	}

	err = db.SaveMCPServer(server, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipMCPServer(server)
	sse.PublishMCPServersChanged(server.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventMCPServerCreate,
		fmt.Sprintf("Created MCP server %s", server.Namespace),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"mcp_server_id":   server.Id,
			"namespace":       server.Namespace,
			"user_id":         server.UserId,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, &apiclient.MCPServerCreateResponse{
		Status: true,
		Id:     server.Id,
	})
}

func HandleUpdateMCPServer(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	serverId := r.PathValue("mcp_server_id")

	if !validate.UUID(serverId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid MCP server ID"})
		return
	}

	db := database.GetInstance()
	server, err := db.GetMCPServer(serverId)
	if err != nil || server.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "MCP server not found"})
		return
	}

	cfg := config.GetServerConfig()
	if !cfg.LeafNode {
		if server.UserId != user.Id && !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit this MCP server"})
			return
		}
		if !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to edit MCP servers"})
			return
		}
	}

	request := apiclient.MCPServerUpdateRequest{}
	err = rest.DecodeRequestBody(w, r, &request)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	// Check namespace uniqueness if changing
	if request.Namespace != server.Namespace {
		all, _ := db.GetMCPServers()
		for _, s := range all {
			if !s.IsDeleted && s.Id != server.Id && s.UserId == server.UserId && s.Namespace == request.Namespace {
				rest.WriteResponse(http.StatusConflict, w, r, ErrorResponse{Error: fmt.Sprintf("MCP server with namespace %q already exists", request.Namespace)})
				return
			}
		}
	}

	server.Namespace = request.Namespace
	server.URL = strings.TrimSuffix(request.URL, "/")
	server.Command = request.Command
	server.Args = request.Args
	server.Env = request.Env
	server.AuthType = request.AuthType
	server.Token = request.Token
	server.OAuthClientID = request.OAuthClientID
	server.OAuthTokenURL = request.OAuthTokenURL
	server.OAuthAccessToken = request.OAuthAccessToken
	server.OAuthRefreshToken = request.OAuthRefreshToken
	server.Enabled = request.Enabled
	server.ToolVisibility = request.ToolVisibility
	if server.ToolVisibility == "" {
		server.ToolVisibility = "native"
	}
	server.RemoteSearch = request.RemoteSearch
	server.UpdatedUserId = user.Id
	server.UpdatedAt = hlc.Now()

	err = db.SaveMCPServer(server, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipMCPServer(server)
	sse.PublishMCPServersChanged(server.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventMCPServerUpdate,
		fmt.Sprintf("Updated MCP server %s", server.Namespace),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"mcp_server_id":   server.Id,
			"namespace":       server.Namespace,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteMCPServer(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	serverId := r.PathValue("mcp_server_id")

	if !validate.UUID(serverId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid MCP server ID"})
		return
	}

	db := database.GetInstance()
	server, err := db.GetMCPServer(serverId)
	if err != nil || server.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "MCP server not found"})
		return
	}

	cfg := config.GetServerConfig()
	if !cfg.LeafNode {
		if server.UserId != user.Id && !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete this MCP server"})
			return
		}
		if !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to delete MCP servers"})
			return
		}
	}

	namespace := server.Namespace
	server.Namespace = server.Id
	server.IsDeleted = true
	server.UpdatedUserId = user.Id
	server.UpdatedAt = hlc.Now()

	err = db.SaveMCPServer(server, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipMCPServer(server)
	sse.PublishMCPServersDeleted(server.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventMCPServerDelete,
		fmt.Sprintf("Deleted MCP server %s", namespace),
		&map[string]interface{}{
			"agent":           r.UserAgent(),
			"IP":              r.RemoteAddr,
			"X-Forwarded-For": r.Header.Get("X-Forwarded-For"),
			"mcp_server_id":   server.Id,
			"namespace":       namespace,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func HandleToggleMCPServerTool(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	serverId := r.PathValue("mcp_server_id")

	if !validate.UUID(serverId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid MCP server ID"})
		return
	}

	db := database.GetInstance()
	server, err := db.GetMCPServer(serverId)
	if err != nil || server.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "MCP server not found"})
		return
	}

	cfg := config.GetServerConfig()
	if !cfg.LeafNode {
		if server.UserId != user.Id && !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to modify this MCP server"})
			return
		}
		if !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to modify MCP servers"})
			return
		}
	}

	request := apiclient.MCPServerToggleToolRequest{}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid request body"})
		return
	}

	if request.ToolName == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "tool_name is required"})
		return
	}

	// Update disabled tools list
	disabledSet := make(map[string]bool)
	for _, t := range server.DisabledTools {
		disabledSet[t] = true
	}

	if request.Enabled {
		delete(disabledSet, request.ToolName)
	} else {
		disabledSet[request.ToolName] = true
	}

	server.DisabledTools = make([]string, 0, len(disabledSet))
	for t := range disabledSet {
		server.DisabledTools = append(server.DisabledTools, t)
	}
	server.UpdatedUserId = user.Id
	server.UpdatedAt = hlc.Now()

	err = db.SaveMCPServer(server, nil)
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	service.GetTransport().GossipMCPServer(server)
	sse.PublishMCPServersChanged(server.Id)

	w.WriteHeader(http.StatusOK)
}

func HandleListMCPServerTools(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	serverId := r.PathValue("mcp_server_id")

	if !validate.UUID(serverId) {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid MCP server ID"})
		return
	}

	db := database.GetInstance()
	server, err := db.GetMCPServer(serverId)
	if err != nil || server.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "MCP server not found"})
		return
	}

	cfg := config.GetServerConfig()
	if !cfg.LeafNode {
		if server.UserId != user.Id && !user.HasPermission(model.PermissionManageMCPServers) {
			rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to view this MCP server"})
			return
		}
	}

	tools, err := internalmcp.ListRemoteServerTools(server)
	if err != nil {
		rest.WriteResponse(http.StatusOK, w, r, map[string]interface{}{"tools": []interface{}{}, "error": err.Error()})
		return
	}

	disabledSet := make(map[string]bool)
	for _, t := range server.DisabledTools {
		disabledSet[t] = true
	}

	result := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		toolName := tool.Name
		if server.Namespace != "" {
			toolName = strings.TrimPrefix(tool.Name, server.Namespace+".")
		}
		result = append(result, map[string]interface{}{
			"name":        toolName,
			"description": tool.Description,
			"enabled":     !disabledSet[toolName],
		})
	}

	rest.WriteResponse(http.StatusOK, w, r, map[string]interface{}{"tools": result})
}

func toMCPServerInfo(s *model.MCPServer) apiclient.MCPServerInfo {
	return apiclient.MCPServerInfo{
		Id:             s.Id,
		UserId:         s.UserId,
		Namespace:      s.Namespace,
		URL:            s.URL,
		Command:        s.Command,
		Args:           s.Args,
		Env:            s.Env,
		Enabled:        s.Enabled,
		ToolVisibility: s.ToolVisibility,
		DisabledTools:  s.DisabledTools,
		RemoteSearch:   s.RemoteSearch,
	}
}

func toMCPServerDetails(s *model.MCPServer) apiclient.MCPServerDetails {
	return apiclient.MCPServerDetails{
		Id:                s.Id,
		UserId:            s.UserId,
		Namespace:         s.Namespace,
		URL:               s.URL,
		Command:           s.Command,
		Args:              s.Args,
		Env:               s.Env,
		AuthType:          s.AuthType,
		Token:             s.Token,
		OAuthClientID:     s.OAuthClientID,
		OAuthTokenURL:     s.OAuthTokenURL,
		OAuthAccessToken:  s.OAuthAccessToken,
		OAuthRefreshToken: s.OAuthRefreshToken,
		Enabled:           s.Enabled,
		ToolVisibility:    s.ToolVisibility,
		DisabledTools:     s.DisabledTools,
		RemoteSearch:      s.RemoteSearch,
	}
}
