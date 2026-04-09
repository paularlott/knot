package model

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/internal/config"
)

// Define actor types
const (
	AuditActorSystem = "System"

	AuditActorTypeUser   = "User"
	AuditActorTypeSystem = "System"
	AuditActorTypeMCP    = "MCP"
)

// Define events
const (
	AuditEventSystemStart = "System Start"

	// Auth
	AuditEventAuthFailed = "Login Failed"
	AuditEventAuthOk     = "Login Success"

	// Groups
	AuditEventGroupCreate = "Group Create"
	AuditEventGroupUpdate = "Group Update"
	AuditEventGroupDelete = "Group Delete"

	// Roles
	AuditEventRoleCreate = "Role Create"
	AuditEventRoleUpdate = "Role Update"
	AuditEventRoleDelete = "Role Delete"

	// Spaces
	AuditEventSpaceCreate    = "Space Create"
	AuditEventSpaceUpdate    = "Space Update"
	AuditEventSpaceDelete    = "Space Delete"
	AuditEventSpaceTransfer  = "Space Transfer"
	AuditEventSpaceShare     = "Space Shared"
	AuditEventSpaceStopShare = "Space Stop Share"

	// Templates
	AuditEventTemplateCreate = "Template Create"
	AuditEventTemplateUpdate = "Template Update"
	AuditEventTemplateDelete = "Template Delete"

	// Variables
	AuditEventVarCreate = "Variable Create"
	AuditEventVarUpdate = "Variable Update"
	AuditEventVarDelete = "Variable Delete"

	// Users
	AuditEventUserCreate = "User Create"
	AuditEventUserUpdate = "User Update"
	AuditEventUserDelete = "User Delete"

	// Volumes
	AuditEventVolumeCreate = "Volume Create"
	AuditEventVolumeUpdate = "Volume Update"
	AuditEventVolumeDelete = "Volume Delete"

	// Scripts
	AuditEventScriptCreate  = "Script Create"
	AuditEventScriptUpdate  = "Script Update"
	AuditEventScriptDelete  = "Script Delete"
	AuditEventScriptExecute = "Script Execute"

	// Skills
	AuditEventSkillCreate = "Skill Create"
	AuditEventSkillUpdate = "Skill Update"
	AuditEventSkillDelete = "Skill Delete"
)

type AuditLogFilter struct {
	Query     string
	Actor     string
	ActorType string
	Event     string
	From      *time.Time
	To        *time.Time
}

type AuditLogEntry struct {
	Id         int64                  `json:"audit_log_id" db:"audit_log_id,pk"`
	Zone       string                 `json:"zone" db:"zone"`
	Actor      string                 `json:"actor" db:"actor"`
	ActorType  string                 `json:"actor_type" db:"actor_type"`
	Event      string                 `json:"event" db:"event"`
	When       time.Time              `json:"created_at" db:"created_at"`
	Details    string                 `json:"details" db:"details"`
	Properties map[string]interface{} `json:"properties" db:"properties,json"`
}

func NewAuditLogEntry(actor, actorType, event, details string, properties *map[string]interface{}) *AuditLogEntry {
	cfg := config.GetServerConfig()

	entry := &AuditLogEntry{
		Zone:      cfg.Zone,
		Actor:     actor,
		ActorType: actorType,
		Event:     event,
		When:      time.Now().UTC(),
		Details:   details,
	}

	if properties != nil {
		entry.Properties = *properties
	}

	return entry
}

// RequestProperties extracts source_ip and user_agent from an HTTP request
// and merges them into an existing properties map, or creates a new one.
func RequestProperties(r *http.Request, properties *map[string]interface{}) *map[string]interface{} {
	if r == nil {
		return properties
	}

	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.RemoteAddr
	}
	if strings.Contains(ip, ",") {
		ip = strings.TrimSpace(strings.Split(ip, ",")[0])
	}
	if ipOnly, _, err := net.SplitHostPort(ip); err == nil {
		ip = ipOnly
	}

	ua := r.UserAgent()

	if properties == nil {
		p := map[string]interface{}{}
		properties = &p
	}

	(*properties)["source_ip"] = ip
	(*properties)["user_agent"] = ua

	return properties
}

// MatchesFilter returns true if the entry matches the given filter criteria.
func (e *AuditLogEntry) MatchesFilter(filter *AuditLogFilter) bool {
	if filter == nil {
		return true
	}
	if filter.Actor != "" && !strings.EqualFold(e.Actor, filter.Actor) {
		return false
	}
	if filter.ActorType != "" && !strings.EqualFold(e.ActorType, filter.ActorType) {
		return false
	}
	if filter.Event != "" && !strings.Contains(strings.ToLower(e.Event), strings.ToLower(filter.Event)) {
		return false
	}
	if filter.From != nil && e.When.Before(*filter.From) {
		return false
	}
	if filter.To != nil && e.When.After(*filter.To) {
		return false
	}
	if filter.Query != "" {
		q := strings.ToLower(filter.Query)
		matched := strings.Contains(strings.ToLower(e.Actor), q) ||
			strings.Contains(strings.ToLower(e.Event), q) ||
			strings.Contains(strings.ToLower(e.Details), q)
		if !matched && e.Properties != nil {
			if propJSON, err := json.Marshal(e.Properties); err == nil {
				matched = strings.Contains(strings.ToLower(string(propJSON)), q)
			}
		}
		if !matched {
			return false
		}
	}
	return true
}
