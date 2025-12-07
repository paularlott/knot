package api_utils

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/validate"
)

// GetSpaceDetails returns detailed space information with permission checks
func GetSpaceDetails(spaceId string, user *model.User) (*apiclient.SpaceDefinition, error) {
	if spaceId == "" {
		return nil, fmt.Errorf("space_id is required")
	}

	if !validate.UUID(spaceId) {
		return nil, fmt.Errorf("Invalid space ID")
	}

	if !user.HasPermission(model.PermissionUseSpaces) {
		return nil, fmt.Errorf("No permission to access spaces")
	}

	db := database.GetInstance()
	space, err := db.GetSpace(spaceId)
	if err != nil || space == nil {
		return nil, fmt.Errorf("Space not found: %v", err)
	}

	// Check ownership or management permissions
	if space.UserId != user.Id && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to access this space")
	}

	// Get template for additional info
	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		return nil, fmt.Errorf("Template not found: %v", err)
	}

	// Get owner username
	owner, err := db.GetUser(space.UserId)
	if err != nil {
		return nil, fmt.Errorf("User not found: %v", err)
	}

	// Format the creation date in the user's timezone
	var createdAtFormatted string
	if user.Timezone != "" {
		if loc, err := time.LoadLocation(user.Timezone); err == nil {
			createdAtFormatted = space.CreatedAt.In(loc).Format("2 / Jan / 2006 3:04:05pm")
		} else {
			createdAtFormatted = space.CreatedAt.UTC().Format("2 / Jan / 2006 3:04:05pm")
		}
	} else {
		createdAtFormatted = space.CreatedAt.UTC().Format("2 / Jan / 2006 3:04:05pm")
	}

	// Get shared user info
	var sharedUserId, sharedUsername string
	if space.SharedWithUserId != "" {
		sharedUser, err := db.GetUser(space.SharedWithUserId)
		if err == nil {
			sharedUserId = sharedUser.Id
			sharedUsername = sharedUser.Username
		}
	}

	// Get agent state
	cfg := config.GetServerConfig()
	state := agent_server.GetSession(space.Id)
	var hasCodeServer, hasSSH, hasTerminal, hasHttpVNC, hasVSCodeTunnel, hasState bool
	var tcpPorts, httpPorts map[string]string
	var vscodeTunnel string

	if state == nil {
		hasCodeServer = false
		hasSSH = false
		hasTerminal = false
		hasHttpVNC = false
		tcpPorts = make(map[string]string)
		httpPorts = make(map[string]string)
		hasVSCodeTunnel = false
		vscodeTunnel = ""
		hasState = false
	} else {
		hasCodeServer = state.HasCodeServer
		hasSSH = state.SSHPort > 0
		hasTerminal = state.HasTerminal
		hasHttpVNC = state.VNCHttpPort > 0
		tcpPorts = state.TcpPorts
		hasState = true

		if cfg.WildcardDomain == "" {
			httpPorts = make(map[string]string)
		} else {
			httpPorts = state.HttpPorts
		}

		hasVSCodeTunnel = state.HasVSCodeTunnel
		vscodeTunnel = state.VSCodeTunnelName
	}

	// Check if template has been updated
	var updateAvailable bool
	if template.IsManual() || template.Hash == "" {
		updateAvailable = false
	} else {
		updateAvailable = space.IsDeployed && space.TemplateHash != template.Hash
	}

	// Check if remote
	isRemote := space.Zone != "" && space.Zone != cfg.Zone

	response := &apiclient.SpaceDefinition{
		SpaceId:            space.Id,
		UserId:             space.UserId,
		TemplateId:         space.TemplateId,
		SharedUserId:       sharedUserId,
		SharedUsername:     sharedUsername,
		Name:               space.Name,
		Description:        space.Description,
		Note:               space.Note,
		TemplateName:       template.Name,
		Username:           owner.Username,
		Platform:           template.Platform,
		Shell:              space.Shell,
		Zone:               space.Zone,
		AltNames:           space.AltNames,
		IsDeployed:         space.IsDeployed,
		IsPending:          space.IsPending,
		IsDeleting:         space.IsDeleting,
		VolumeData:         space.VolumeData,
		StartedAt:          space.StartedAt.UTC(),
		CreatedAt:          space.CreatedAt.UTC(),
		CreatedAtFormatted: createdAtFormatted,
		IconURL:            space.IconURL,
		CustomFields:       make([]apiclient.CustomFieldValue, len(space.CustomFields)),
		HasCodeServer:      hasCodeServer,
		HasSSH:             hasSSH,
		HasTerminal:        hasTerminal,
		HasHttpVNC:         hasHttpVNC,
		HasState:           hasState,
		TcpPorts:           tcpPorts,
		HttpPorts:          httpPorts,
		UpdateAvailable:    updateAvailable,
		HasVSCodeTunnel:    hasVSCodeTunnel,
		VSCodeTunnel:       vscodeTunnel,
		IsRemote:           isRemote,
	}

	for i, field := range space.CustomFields {
		response.CustomFields[i] = apiclient.CustomFieldValue{
			Name:  field.Name,
			Value: field.Value,
		}
	}

	return response, nil
}
