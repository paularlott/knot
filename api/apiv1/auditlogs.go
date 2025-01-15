package apiv1

import (
	"net/http"
	"strconv"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/util/rest"
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

	// If remote client present then forward the request
	remoteClient := r.Context().Value("remote_client")
	if remoteClient != nil {
		client := remoteClient.(*apiclient.ApiClient)

		auditLogs, code, err := client.GetAuditLogs(start, maxItems)
		if err != nil {
			rest.SendJSON(code, w, r, ErrorResponse{Error: err.Error()})
			return
		}

		rest.SendJSON(http.StatusOK, w, r, auditLogs)
	} else {
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
			auditLogs.Items[i] = apiclient.AuditLogEntry{
				Id:         log.Id,
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
}
