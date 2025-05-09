package service

import (
	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/database/model"
)

type Transport interface {
	GossipGroup(group *model.Group)
	GossipRole(role *model.Role)
	GossipSpace(space *model.Space)
	GossipTemplate(template *model.Template)
	GossipTemplateVar(templateVar *model.TemplateVar)
	GossipUser(user *model.User)
	GossipVolume(volume *model.Volume)

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
