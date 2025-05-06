package service

import "github.com/paularlott/knot/database/model"

type Transport interface {
	GossipGroup(group *model.Group)
	GossipRole(role *model.Role)
	GossipSpace(space *model.Space)

	GossipUser(user *model.User)
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
