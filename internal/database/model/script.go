package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type Script struct {
	Id                  string        `json:"script_id" db:"script_id,pk"`
	Name                string        `json:"name" db:"name"`
	Description         string        `json:"description" db:"description"`
	Content             string        `json:"content" db:"content"`
	Groups              []string      `json:"groups" db:"groups,json"`
	Active              bool          `json:"active" db:"active"`
	ScriptType          string        `json:"script_type" db:"script_type"`
	MCPInputSchemaToml  string        `json:"mcp_input_schema_toml" db:"mcp_input_schema_toml"`
	MCPKeywords         []string      `json:"mcp_keywords" db:"mcp_keywords,json"`
	Timeout             int           `json:"timeout" db:"timeout"`
	IsDeleted           bool          `json:"is_deleted" db:"is_deleted"`
	CreatedUserId       string        `json:"created_user_id" db:"created_user_id"`
	CreatedAt           time.Time     `json:"created_at" db:"created_at"`
	UpdatedUserId       string        `json:"updated_user_id" db:"updated_user_id"`
	UpdatedAt           hlc.Timestamp `json:"updated_at" db:"updated_at"`
}

func NewScript(
	name string,
	description string,
	content string,
	groups []string,
	active bool,
	scriptType string,
	mcpInputSchemaToml string,
	mcpKeywords []string,
	timeout int,
	userId string,
) *Script {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	if scriptType == "" {
		scriptType = "script"
	}

	return &Script{
		Id:                 id.String(),
		Name:               name,
		Description:        description,
		Content:            content,
		Groups:             groups,
		Active:             active,
		ScriptType:         scriptType,
		MCPInputSchemaToml: mcpInputSchemaToml,
		MCPKeywords:        mcpKeywords,
		Timeout:            timeout,
		CreatedUserId:      userId,
		CreatedAt:          time.Now().UTC(),
		UpdatedUserId:      userId,
		UpdatedAt:          hlc.Now(),
	}
}
