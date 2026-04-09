package api

import (
	"encoding/csv"
	"net/http"
	"strconv"
	"time"

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
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	totalLogs, err := db.GetNumberOfAuditLogs()
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
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

	rest.WriteResponse(http.StatusOK, w, r, auditLogs)
}

func HandleExportAuditLogs(w http.ResponseWriter, r *http.Request) {
	var from, to *time.Time

	if v := r.URL.Query().Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			from = &t
		}
	}
	if v := r.URL.Query().Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			to = &t
		}
	}

	db := database.GetInstance()
	logs, err := db.GetAuditLogsForExport(from, to)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}

	format := r.URL.Query().Get("format")
	if format == "json" {
		items := make([]apiclient.AuditLogEntry, len(logs))
		for i, entry := range logs {
			if entry.Id == 0 {
				entry.Id = entry.When.UnixMicro()
			}
			items[i] = apiclient.AuditLogEntry{
				Id:         entry.Id,
				Zone:       entry.Zone,
				When:       entry.When,
				Actor:      entry.Actor,
				ActorType:  entry.ActorType,
				Event:      entry.Event,
				Details:    entry.Details,
				Properties: entry.Properties,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.json"`)
		rest.WriteResponse(http.StatusOK, w, r, items)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="audit-logs.csv"`)
	csvWriter := csv.NewWriter(w)
	_ = csvWriter.Write([]string{"time", "zone", "actor", "actor_type", "event", "details"})
	for _, entry := range logs {
		_ = csvWriter.Write([]string{
			entry.When.UTC().Format(time.RFC3339),
			entry.Zone,
			entry.Actor,
			entry.ActorType,
			entry.Event,
			entry.Details,
		})
	}
	csvWriter.Flush()
}
