package cluster

import "github.com/paularlott/gossip"

const (
	GroupFullSyncMsg gossip.MessageType = iota + gossip.UserMsg
	GroupGossipMsg
	RoleFullSyncMsg
	RoleGossipMsg
	SpaceFullSyncMsg
	SpaceGossipMsg

	UserFullSyncMsg
	UserGossipMsg
)
