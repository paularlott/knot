package api

import (
	"net/http"
	"strconv"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/util/rest"
)

func HandleGetAuditLogs(w http.ResponseWriter, r *http.Request) {
	startParam := r.URL.Query().Get("start")
	maxItemsParam := r.URL.Query().Get("max-items")

	start, err := strconv.Atoi(startParam)
	if err != nil {
		start = 0
	}

	maxItems, err := strconv.Atoi(maxItemsParam)
	if err != nil {
		maxItems = 10
	}

	db := database.GetInstance()
	logs, err := db.GetAuditLogs(start, maxItems)
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	totalLogs, err := db.GetNumberOfAuditLogs()
	if err != nil {
		rest.SendJSON(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	auditLogs := apiclient.AuditLogs{
		Count: totalLogs,
		Items: make([]apiclient.AuditLogEntry, len(logs)),
	}
	for i, log := range logs {
		if log.Id == 0 {
			log.Id = log.When.UnixMicro()
		}

		auditLogs.Items[i] = apiclient.AuditLogEntry{
			Id:         log.Id,
			Zone:       log.Zone,
			When:       log.When,
			Actor:      log.Actor,
			ActorType:  log.ActorType,
			Event:      log.Event,
			Details:    log.Details,
			Properties: log.Properties,
		}
	}

	rest.SendJSON(http.StatusOK, w, r, auditLogs)
}
