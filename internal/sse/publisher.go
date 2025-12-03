package sse

// Publish functions for broadcasting events to SSE clients.
// These should be called after data modifications alongside gossip calls.

// PublishGroupsChanged notifies clients that groups have changed
func PublishGroupsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventGroupsChanged,
	})
}

// PublishRolesChanged notifies clients that roles have changed
func PublishRolesChanged() {
	GetHub().Broadcast(&Event{
		Type: EventRolesChanged,
	})
}

// PublishTemplatesChanged notifies clients that templates have changed
func PublishTemplatesChanged() {
	GetHub().Broadcast(&Event{
		Type: EventTemplatesChanged,
	})
}

// PublishTemplateVarsChanged notifies clients that template variables have changed
func PublishTemplateVarsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventTemplateVarsChanged,
	})
}

// PublishUsersChanged notifies clients that users have changed
func PublishUsersChanged() {
	GetHub().Broadcast(&Event{
		Type: EventUsersChanged,
	})
}

// PublishTokensChanged notifies clients that tokens have changed
func PublishTokensChanged() {
	GetHub().Broadcast(&Event{
		Type: EventTokensChanged,
	})
}

// PublishVolumesChanged notifies clients that volumes have changed
func PublishVolumesChanged() {
	GetHub().Broadcast(&Event{
		Type: EventVolumesChanged,
	})
}

// PublishSessionsChanged notifies clients that sessions have changed
func PublishSessionsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventSessionsChanged,
	})
}

// PublishTunnelsChanged notifies clients that tunnels have changed
func PublishTunnelsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventTunnelsChanged,
	})
}

// PublishAuditLogsChanged notifies clients that audit logs have changed
func PublishAuditLogsChanged() {
	GetHub().Broadcast(&Event{
		Type: EventAuditLogsChanged,
	})
}

// PublishSpaceCreated notifies clients that a space was created
func PublishSpaceCreated(spaceId, userId string) {
	GetHub().Broadcast(&Event{
		Type: EventSpaceCreated,
		Payload: SpaceEventPayload{
			SpaceId: spaceId,
			UserId:  userId,
		},
	})
}

// PublishSpaceUpdated notifies clients that a space was updated
func PublishSpaceUpdated(spaceId, userId string) {
	GetHub().Broadcast(&Event{
		Type: EventSpaceUpdated,
		Payload: SpaceEventPayload{
			SpaceId: spaceId,
			UserId:  userId,
		},
	})
}

// PublishSpaceDeleted notifies clients that a space was deleted
func PublishSpaceDeleted(spaceId, userId string) {
	GetHub().Broadcast(&Event{
		Type: EventSpaceDeleted,
		Payload: SpaceEventPayload{
			SpaceId: spaceId,
			UserId:  userId,
		},
	})
}

// PublishSpaceStateChanged notifies clients that a space's deployment state changed
func PublishSpaceStateChanged(spaceId, userId string) {
	GetHub().Broadcast(&Event{
		Type: EventSpaceState,
		Payload: SpaceEventPayload{
			SpaceId: spaceId,
			UserId:  userId,
		},
	})
}
