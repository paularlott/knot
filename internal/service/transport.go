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
	GossipSpaceUsageSample(sample *model.SpaceUsageSample)
	GossipAuditLog(entry *model.AuditLogEntry)
	GossipSession(session *model.Session)
	GossipScript(script *model.Script)
	GossipSkill(skill *model.Skill)
	GossipEventSink(sink *model.EventSink)
	GossipStackDefinition(stackDef *model.StackDefinition)
	GossipResponse(response *model.Response)
	GossipPoolDefinition(pool *model.PoolDefinition)
	GossipPoolDrain(spaceID string)
	GossipPoolUndrain(spaceID string)
	BroadcastEvent(envelope *EventEnvelope)
	GetAgentEndpoints() []string
	GetTunnelServers() []string
	IsLeader() bool

	LockResource(resourceId string) string
	UnlockResource(resourceId, unlockToken string)

	Nodes() []*gossip.Node
	GetNodeByIDString(id string) *gossip.Node
	EnqueueSpaceCleanup(space *model.Space)
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
