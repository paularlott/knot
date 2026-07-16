package model

import (
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type MCPServer struct {
	Id                string        `json:"mcp_server_id" db:"mcp_server_id,pk" msgpack:"mcp_server_id"`
	UserId            string        `json:"user_id" db:"user_id" msgpack:"user_id"`
	Namespace         string        `json:"namespace" db:"namespace" msgpack:"namespace"`
	URL               string        `json:"url" db:"url" msgpack:"url"`
	Command           string        `json:"command" db:"command" msgpack:"command"`
	Args              []string      `json:"args" db:"args,json" msgpack:"args"`
	Env               []string      `json:"env" db:"env,json" msgpack:"env"`
	AuthType          string        `json:"auth_type" db:"auth_type" msgpack:"auth_type"`
	Token             string        `json:"token" db:"token" msgpack:"token"`
	OAuthClientID     string        `json:"oauth_client_id" db:"oauth_client_id" msgpack:"oauth_client_id"`
	OAuthTokenURL     string        `json:"oauth_token_url" db:"oauth_token_url" msgpack:"oauth_token_url"`
	OAuthAccessToken  string        `json:"oauth_access_token" db:"oauth_access_token" msgpack:"oauth_access_token"`
	OAuthRefreshToken string        `json:"oauth_refresh_token" db:"oauth_refresh_token" msgpack:"oauth_refresh_token"`
	Enabled           bool          `json:"enabled" db:"enabled" msgpack:"enabled"`
	ToolVisibility    string        `json:"tool_visibility" db:"tool_visibility" msgpack:"tool_visibility"`
	DisabledTools     []string      `json:"disabled_tools" db:"disabled_tools,json" msgpack:"disabled_tools"`
	RemoteSearch      bool          `json:"remote_search" db:"remote_search" msgpack:"remote_search"`
	IsDeleted         bool          `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	CreatedUserId     string        `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt         time.Time     `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId     string        `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt         hlc.Timestamp `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

func NewMCPServer(
	namespace string,
	ownerUserId string,
	createdUserId string,
) *MCPServer {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	return &MCPServer{
		Id:            id.String(),
		UserId:        ownerUserId,
		Namespace:     namespace,
		Enabled:       true,
		ToolVisibility: "native",
		CreatedUserId: createdUserId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: createdUserId,
		UpdatedAt:     hlc.Now(),
	}
}

func (s *MCPServer) IsUserServer() bool {
	return s.UserId != ""
}

// IsToolEnabled checks if a specific tool is enabled for this server.
// A tool is enabled unless it appears in the DisabledTools list.
func (s *MCPServer) IsToolEnabled(toolName string) bool {
	if len(s.DisabledTools) > 0 && slices.Contains(s.DisabledTools, toolName) {
		return false
	}
	return true
}
