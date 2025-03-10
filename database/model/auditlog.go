package model

import "time"

// Define actor types
const (
	AuditActorSystem = "System"

	AuditActorTypeUser   = "User"
	AuditActorTypeSystem = "System"
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
)

type AuditLogEntry struct {
	Id         int64                  `json:"audit_log_id"`
	Actor      string                 `json:"actor"`
	ActorType  string                 `json:"actor_type"`
	Event      string                 `json:"event"`
	When       time.Time              `json:"created_at"`
	Details    string                 `json:"details"`
	Properties map[string]interface{} `json:"properties"`
}

func NewAuditLogEntry(actor, actorType, event, details string, properties *map[string]interface{}) *AuditLogEntry {
	entry := &AuditLogEntry{
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
