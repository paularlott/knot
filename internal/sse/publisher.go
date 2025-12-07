package sse

// Publish functions for broadcasting events to SSE clients.
// These should be called after data modifications alongside gossip calls.

// PublishGroupsChanged notifies clients that a group has changed
func PublishGroupsChanged(groupId string) {
	GetHub().Broadcast(&Event{
		Type:    EventGroupsChanged,
		Payload: ResourcePayload{Id: groupId},
	})
}

// PublishGroupsDeleted notifies clients that a group was deleted
func PublishGroupsDeleted(groupId string) {
	GetHub().Broadcast(&Event{
		Type:    EventGroupsDeleted,
		Payload: ResourcePayload{Id: groupId},
	})
}

// PublishRolesChanged notifies clients that a role has changed
func PublishRolesChanged(roleId string) {
	GetHub().Broadcast(&Event{
		Type:    EventRolesChanged,
		Payload: ResourcePayload{Id: roleId},
	})
}

// PublishRolesDeleted notifies clients that a role was deleted
func PublishRolesDeleted(roleId string) {
	GetHub().Broadcast(&Event{
		Type:    EventRolesDeleted,
		Payload: ResourcePayload{Id: roleId},
	})
}

// PublishTemplatesChanged notifies clients that a template has changed
func PublishTemplatesChanged(templateId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTemplatesChanged,
		Payload: ResourcePayload{Id: templateId},
	})
}

// PublishTemplatesDeleted notifies clients that a template was deleted
func PublishTemplatesDeleted(templateId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTemplatesDeleted,
		Payload: ResourcePayload{Id: templateId},
	})
}

// PublishTemplateVarsChanged notifies clients that a template variable has changed
func PublishTemplateVarsChanged(templateVarId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTemplateVarsChanged,
		Payload: ResourcePayload{Id: templateVarId},
	})
}

// PublishTemplateVarsDeleted notifies clients that a template variable was deleted
func PublishTemplateVarsDeleted(templateVarId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTemplateVarsDeleted,
		Payload: ResourcePayload{Id: templateVarId},
	})
}

// PublishUsersChanged notifies clients that a user has changed
func PublishUsersChanged(userId string) {
	GetHub().Broadcast(&Event{
		Type:    EventUsersChanged,
		Payload: ResourcePayload{Id: userId},
	})
}

// PublishUsersDeleted notifies clients that a user was deleted
func PublishUsersDeleted(userId string) {
	GetHub().Broadcast(&Event{
		Type:    EventUsersDeleted,
		Payload: ResourcePayload{Id: userId},
	})
}

// PublishTokensChanged notifies clients that a token has changed
func PublishTokensChanged(tokenId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTokensChanged,
		Payload: ResourcePayload{Id: tokenId},
	})
}

// PublishTokensDeleted notifies clients that a token was deleted
func PublishTokensDeleted(tokenId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTokensDeleted,
		Payload: ResourcePayload{Id: tokenId},
	})
}

// PublishVolumesChanged notifies clients that a volume has changed
func PublishVolumesChanged(volumeId string) {
	GetHub().Broadcast(&Event{
		Type:    EventVolumesChanged,
		Payload: ResourcePayload{Id: volumeId},
	})
}

// PublishVolumesDeleted notifies clients that a volume was deleted
func PublishVolumesDeleted(volumeId string) {
	GetHub().Broadcast(&Event{
		Type:    EventVolumesDeleted,
		Payload: ResourcePayload{Id: volumeId},
	})
}

// PublishSessionsChanged notifies clients that a session has changed
func PublishSessionsChanged(sessionId string) {
	GetHub().Broadcast(&Event{
		Type:    EventSessionsChanged,
		Payload: ResourcePayload{Id: sessionId},
	})
}

// PublishSessionsDeleted notifies clients that a session was deleted
func PublishSessionsDeleted(sessionId string) {
	GetHub().Broadcast(&Event{
		Type:    EventSessionsDeleted,
		Payload: ResourcePayload{Id: sessionId},
	})
}

// PublishTunnelsChanged notifies clients that a tunnel has changed
func PublishTunnelsChanged(tunnelId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTunnelsChanged,
		Payload: ResourcePayload{Id: tunnelId},
	})
}

// PublishTunnelsDeleted notifies clients that a tunnel was deleted
func PublishTunnelsDeleted(tunnelId string) {
	GetHub().Broadcast(&Event{
		Type:    EventTunnelsDeleted,
		Payload: ResourcePayload{Id: tunnelId},
	})
}

// PublishAuditLogsChanged notifies clients that audit logs have changed
func PublishAuditLogsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventAuditLogsChanged,
	})
}

// PublishSpaceChanged notifies clients that a space was created, updated, or state changed
// Parameters: spaceId, userId, optional sharedWithUserId, optional previousUserId
func PublishSpaceChanged(spaceId, userId string, optionalIds ...string) {
	payload := ResourcePayload{
		Id:     spaceId,
		UserId: userId,
	}
	if len(optionalIds) > 0 {
		payload.SharedWithUserId = optionalIds[0]
	}
	if len(optionalIds) > 1 {
		payload.PreviousUserId = optionalIds[1]
	}
	GetHub().Broadcast(&Event{
		Type:    EventSpaceChanged,
		Payload: payload,
	})
}

// PublishSpaceDeleted notifies clients that a space was deleted
func PublishSpaceDeleted(spaceId, userId string) {
	GetHub().Broadcast(&Event{
		Type: EventSpaceDeleted,
		Payload: ResourcePayload{
			Id:     spaceId,
			UserId: userId,
		},
	})
}
