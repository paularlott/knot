package cluster

import "github.com/paularlott/gossip"

const (
	GroupFullSyncMsg gossip.MessageType = iota + gossip.UserMsg
	GroupGossipMsg
	RoleFullSyncMsg
	RoleGossipMsg
	SpaceFullSyncMsg
	SpaceGossipMsg
	TemplateFullSyncMsg
	TemplateGossipMsg
	TemplateVarFullSyncMsg
	TemplateVarGossipMsg
	UserFullSyncMsg
	UserGossipMsg
	TokenFullSyncMsg
	TokenGossipMsg
	SessionFullSyncMsg
	SessionGossipMsg
	VolumeFullSyncMsg
	VolumeGossipMsg
	AuditLogGossipMsg
)
