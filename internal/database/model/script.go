package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type Script struct {
	Id                  string        `json:"script_id" db:"script_id,pk"`
	UserId              string        `json:"user_id" db:"user_id"`
	Name                string        `json:"name" db:"name"`
	Description         string        `json:"description" db:"description"`
	Content             string        `json:"content" db:"content"`
	Groups              []string      `json:"groups" db:"groups,json"`
	Zones               []string      `json:"zones" db:"zones,json"`
	Active              bool          `json:"active" db:"active"`
	ScriptType          string        `json:"script_type" db:"script_type"`
	MCPInputSchemaToml  string        `json:"mcp_input_schema_toml" db:"mcp_input_schema_toml"`
	MCPKeywords         []string      `json:"mcp_keywords" db:"mcp_keywords,json"`
	IsDeleted           bool          `json:"is_deleted" db:"is_deleted"`
	IsManaged           bool          `json:"is_managed" db:"is_managed"`
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
	zones []string,
	active bool,
	scriptType string,
	mcpInputSchemaToml string,
	mcpKeywords []string,
	ownerUserId string,
	createdUserId string,
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
		UserId:             ownerUserId,
		Name:               name,
		Description:        description,
		Content:            content,
		Groups:             groups,
		Zones:              zones,
		Active:             active,
		ScriptType:         scriptType,
		MCPInputSchemaToml: mcpInputSchemaToml,
		MCPKeywords:        mcpKeywords,
		CreatedUserId:      createdUserId,
		CreatedAt:          time.Now().UTC(),
		UpdatedUserId:      createdUserId,
		UpdatedAt:          hlc.Now(),
	}
}


// IsValidForZone determines whether the script is valid for execution in the specified zone.
// The function evaluates zone restrictions based on the script's Zones configuration.
// If no zones are specified, the script is considered valid for all zones (global).
// Zone names prefixed with '!' are treated as exclusions (negated zones).
// The function first checks for exclusions, then checks for explicit inclusions.
//
// zone is the target zone name to validate against the script's zone restrictions.
//
// Returns true if the script can be executed in the specified zone, false otherwise.
func (script *Script) IsValidForZone(zone string) bool {
	// If no zones specified, script is valid for all zones (global)
	if len(script.Zones) == 0 {
		return true
	}

	// Check for negated zones first
	for _, z := range script.Zones {
		if len(z) > 0 && z[0] == '!' && z[1:] == zone {
			return false
		}
	}

	// Check for positive zones
	for _, z := range script.Zones {
		if len(z) > 0 && z[0] != '!' && z == zone {
			return true
		}
	}

	return false
}

// IsGlobalScript returns true if the script is a system/global script (UserId is empty)
func (script *Script) IsGlobalScript() bool {
	return script.UserId == ""
}

// IsUserScript returns true if the script is a user script (UserId is not empty)
func (script *Script) IsUserScript() bool {
	return script.UserId != ""
}
