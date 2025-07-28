package service

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database/model"
)

type Transport interface {
	GossipGroup(group *model.Group)
	GossipRole(role *model.Role)
	GossipSpace(space *model.Space)
	GossipTemplate(template *model.Template)
	GossipTemplateVar(templateVar *model.TemplateVar)
	GossipUser(user *model.User)
	GossipToken(token *model.Token)
	GossipVolume(volume *model.Volume)
	GossipAuditLog(entry *model.AuditLogEntry)
	GossipSession(session *model.Session)
	GetAgentEndpoints() []string
	GetTunnelServers() []string

	LockResource(resourceId string) string
	UnlockResource(resourceId, unlockToken string)

	Nodes() []*gossip.Node
}

var (
	transport Transport
)

func SetTransport(t Transport) {
	transport = t
}

func GetTransport() Transport {
	return transport
}
