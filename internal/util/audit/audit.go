package audit

import (
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

func Log(actor, actorType, event, details string, properties *map[string]interface{}) error {
	entry := model.NewAuditLogEntry(actor, actorType, event, details, properties)
	transport := service.GetTransport()
	if transport != nil {
		transport.GossipAuditLog(entry)
	}
	err := database.GetInstance().SaveAuditLog(entry)
	if err == nil {
		sse.PublishAuditLogsChanged()
	}
	return err
}
