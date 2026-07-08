package service

import (
	"sync"

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
	GossipCommand(command *model.Command)
	GossipEventSink(sink *model.EventSink)
	GossipStackDefinition(stackDef *model.StackDefinition)
	GossipResponse(response *model.Response)
	GossipConversation(conv *model.Conversation)
	GossipPoolDefinition(pool *model.PoolDefinition)
	GossipPoolDrain(spaceID string)
	GossipPoolUndrain(spaceID string)
	BroadcastEvent(envelope *EventEnvelope)
	NotifyEventDone(eventId string)
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
	transport   Transport
	transportMu sync.RWMutex
)

func SetTransport(t Transport) {
	transportMu.Lock()
	transport = t
	transportMu.Unlock()
}

func GetTransport() Transport {
	transportMu.RLock()
	defer transportMu.RUnlock()
	return transport
}
