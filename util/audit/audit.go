package audit

import (
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
)

func Log(actor, actorType, event, details string, properties *map[string]interface{}) error {
	return database.GetInstance().SaveAuditLog(model.NewAuditLogEntry(actor, actorType, event, details, properties))
}
