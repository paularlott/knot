package apiclient

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

type AuditLogEntry struct {
	Id         int64                  `json:"audit_log_id"`
	Zone       string                 `json:"zone"`
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

type AuditLogFilter struct {
	Query     string
	Actor     string
	ActorType string
	Event     string
	From      *time.Time
	To        *time.Time
}

func (c *ApiClient) GetAuditLogs(ctx context.Context, start int, maxItems int, filter *AuditLogFilter) (*AuditLogs, int, error) {
	response := &AuditLogs{}

	params := url.Values{}
	params.Set("start", fmt.Sprintf("%d", start))
	params.Set("max-items", fmt.Sprintf("%d", maxItems))

	if filter != nil {
		if filter.Query != "" {
			params.Set("q", filter.Query)
		}
		if filter.Actor != "" {
			params.Set("actor", filter.Actor)
		}
		if filter.ActorType != "" {
			params.Set("actor_type", filter.ActorType)
		}
		if filter.Event != "" {
			params.Set("event", filter.Event)
		}
		if filter.From != nil {
			params.Set("from", filter.From.Format(time.RFC3339))
		}
		if filter.To != nil {
			params.Set("to", filter.To.Format(time.RFC3339))
		}
	}

	code, err := c.httpClient.Get(ctx, fmt.Sprintf("/api/audit-logs?%s", params.Encode()), response)
	if err != nil {
		return nil, code, err
	}

	return response, code, nil
}
