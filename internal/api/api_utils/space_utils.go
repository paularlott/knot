package api_utils

import (
	"fmt"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/health"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/util/validate"
)

func BuildAPIShares(space *model.Space) []string {
	space.NormalizeShares()

	shares := make([]string, len(space.Shares))
	copy(shares, space.Shares)
	return shares
}

func BuildAPIDependsOn(space *model.Space) []string {
	space.NormalizeDependsOn()

	dependsOn := make([]string, len(space.DependsOn))
	copy(dependsOn, space.DependsOn)
	return dependsOn
}

func GetLatestSpaceResourceUsage(spaceId string) *apiclient.SpaceResourceUsage {
	if spaceId == "" {
		return nil
	}

	now := time.Now().UTC()
	samples, err := database.GetInstance().GetSpaceUsageSamples(spaceId, model.SpaceUsageBucketMinute, now.Add(-model.SpaceUsageMinuteRetention), now)
	if err != nil || len(samples) == 0 {
		return nil
	}

	sample := samples[len(samples)-1]
	return &apiclient.SpaceResourceUsage{
		CPUPercent:       sample.CPUPercent,
		MemoryUsedBytes:  sample.MemoryUsedBytes,
		MemoryLimitBytes: sample.MemoryLimitBytes,
		DiskUsedBytes:    sample.DiskUsedBytes,
		DiskLimitBytes:   sample.DiskLimitBytes,
	}
}

func GetAccessibleSpace(spaceId string, user *model.User) (*model.Space, error) {
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

	if space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		return nil, fmt.Errorf("No permission to access this space")
	}

	return space, nil
}

func getNodeHostname(nodeId string, isRemote bool, fallbackHostname string) string {
	if nodeId == "" {
		return ""
	}

	if transport := service.GetTransport(); transport != nil {
		node := transport.GetNodeByIDString(nodeId)
		if node != nil {
			return node.Metadata.GetString("hostname")
		}
	}

	if isRemote {
		return "Offline Remote Node"
	}
	return fallbackHostname
}

// GetSpaceDetails returns detailed space information with permission checks
func GetSpaceDetails(spaceId string, user *model.User) (*apiclient.SpaceDefinition, error) {
	space, err := GetAccessibleSpace(spaceId, user)
	if err != nil {
		return nil, err
	}

	db := database.GetInstance()

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

	shares := BuildAPIShares(space)
	dependsOn := BuildAPIDependsOn(space)

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

	// Health status from in-memory store
	healthy := true
	if hs := health.Get(space.Id); hs != nil {
		healthy = hs.Healthy
	}

	nodeHostname := getNodeHostname(space.NodeId, isRemote, cfg.Hostname)

	response := &apiclient.SpaceDefinition{
		SpaceId:            space.Id,
		UserId:             space.UserId,
		TemplateId:         space.TemplateId,
		Shares:             shares,
		DependsOn:          dependsOn,
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
		HasEverStarted:     space.TemplateHash != "",
		VolumeData:         space.VolumeData,
		StartedAt:          space.StartedAt.UTC(),
		CreatedAt:          space.CreatedAt.UTC(),
		CreatedAtFormatted: createdAtFormatted,
		IconURL:            space.IconURL,
		CustomFields:       make([]apiclient.CustomFieldValue, len(space.CustomFields)),
		StartupScriptId:    space.StartupScriptId,
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
		NodeId:             space.NodeId,
		NodeHostname:       nodeHostname,
		Healthy:            healthy,
		Stack:              space.Stack,
		StackPrefix:        space.StackPrefix,
		PoolId:             space.PoolId,
		PoolName:           service.PoolNameForSpace(space),
	}

	if state != nil {
		response.ResourceUsage = &apiclient.SpaceResourceUsage{
			CPUPercent:       state.CPUPercent,
			MemoryUsedBytes:  state.MemoryUsedBytes,
			MemoryLimitBytes: state.MemoryLimitBytes,
			DiskUsedBytes:    state.DiskUsedBytes,
			DiskLimitBytes:   state.DiskLimitBytes,
		}
	} else {
		response.ResourceUsage = GetLatestSpaceResourceUsage(space.Id)
	}

	for i, field := range space.CustomFields {
		response.CustomFields[i] = apiclient.CustomFieldValue{
			Name:  field.Name,
			Value: field.Value,
		}
	}

	return response, nil
}
