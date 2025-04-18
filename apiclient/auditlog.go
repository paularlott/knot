package apiclient

import (
	"fmt"
	"time"
)

type AuditLogEntry struct {
	Id         int64                  `json:"audit_log_id"`
	Actor      string                 `json:"actor"`
	ActorType  string                 `json:"actor_type"`
	Event      string                 `json:"event"`
	When       time.Time              `json:"when"`
	Details    string                 `json:"details"`
	Properties map[string]interface{} `json:"properties"`
}

type AuditLogs struct {
	Count int             `json:"count"`
	Items []AuditLogEntry `json:"items"`
}

func (c *ApiClient) GetAuditLogs(start int, maxItems int) (*AuditLogs, int, error) {
	response := &AuditLogs{}

	code, err := c.httpClient.Get(fmt.Sprintf("/api/audit-logs?start=%d&max-items=%d", start, maxItems), response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
