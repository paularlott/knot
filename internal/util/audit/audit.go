package audit

import (
	"net/http"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
)

// LogWithRequest logs an audit event enriched with source IP and user-agent from the HTTP request.
func LogWithRequest(r *http.Request, actor, actorType, event, details string, properties *map[string]interface{}) error {
	return Log(actor, actorType, event, details, model.RequestProperties(r, properties))
}

func Log(actor, actorType, event, details string, properties *map[string]interface{}) error {
	entry := model.NewAuditLogEntry(actor, actorType, event, details, properties)
	transport := service.GetTransport()
	if transport != nil {
		transport.GossipAuditLog(entry)
	}

	cfg := config.GetServerConfig()
	routing := "internal"
	if cfg != nil && cfg.Audit.Routing != "" {
		routing = cfg.Audit.Routing
	}

	if routing == "external" || routing == "both" {
		logToExternal(entry, cfg)
	}

	if routing == "internal" || routing == "both" {
		if database.GetInstance().HasAuditLog() {
			err := database.GetInstance().SaveAuditLog(entry)
			if err == nil {
				sse.PublishAuditLogsChanged()
			}
			return err
		}
	}

	return nil
}

func logToExternal(entry *model.AuditLogEntry, cfg *config.ServerConfig) {
	stream := "audit"
	if cfg != nil && cfg.Audit.AuditStream != "" {
		stream = cfg.Audit.AuditStream
	}

	args := []any{
		"stream", stream,
		"type", "audit",
		"_time", entry.When.UTC().Format(time.RFC3339Nano),
		"zone", entry.Zone,
		"actor", entry.Actor,
		"actor_type", entry.ActorType,
		"event", entry.Event,
		"details", entry.Details,
	}
	if entry.Properties != nil {
		args = append(args, "properties", entry.Properties)
	}
	log.Info(entry.Event, args...)
}
