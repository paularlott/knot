package audit

import (
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
)

func Log(actor, actorType, event, details string, properties *map[string]interface{}) error {
	entry := model.NewAuditLogEntry(actor, actorType, event, details, properties)
	transport := service.GetTransport()
	if transport != nil {
		transport.GossipAuditLog(entry)
	}
	return database.GetInstance().SaveAuditLog(entry)
}
