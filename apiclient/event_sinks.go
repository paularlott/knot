package apiclient

import "context"

type WebhookConfig struct {
	URL           string            `json:"url"`
	Secret        string            `json:"secret"`
	Headers       map[string]string `json:"headers,omitempty"`
	BodyTemplate  string            `json:"body_template"`
	SkipTLSVerify bool              `json:"skip_tls_verify"`
}

type EventSinkInfo struct {
	Id          string         `json:"event_sink_id"`
	UserId      string         `json:"user_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Events      []string       `json:"events"`
	SinkType    string         `json:"sink_type"`
	Webhook     *WebhookConfig `json:"webhook,omitempty"`
	ScriptId    string         `json:"script_id,omitempty"`
	Active      bool           `json:"active"`
}

type EventSinkList struct {
	Count      int             `json:"count"`
	EventSinks []EventSinkInfo `json:"event_sinks"`
}

type EventSinkDetails struct {
	Id          string         `json:"event_sink_id"`
	UserId      string         `json:"user_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Events      []string       `json:"events"`
	SinkType    string         `json:"sink_type"`
	Webhook     *WebhookConfig `json:"webhook,omitempty"`
	ScriptId    string         `json:"script_id,omitempty"`
	Active      bool           `json:"active"`
}

type EventSinkCreateRequest struct {
	UserId      string         `json:"user_id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Events      []string       `json:"events"`
	SinkType    string         `json:"sink_type"`
	Webhook     *WebhookConfig `json:"webhook,omitempty"`
	ScriptId    string         `json:"script_id,omitempty"`
	Active      bool           `json:"active"`
}

type EventSinkUpdateRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Events      []string       `json:"events"`
	SinkType    string         `json:"sink_type"`
	Webhook     *WebhookConfig `json:"webhook,omitempty"`
	ScriptId    string         `json:"script_id,omitempty"`
	Active      bool           `json:"active"`
}

type EventSinkCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"event_sink_id"`
}

type EmitEventRequest struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

func (c *ApiClient) GetEventSinks(ctx context.Context) (*EventSinkList, error) {
	var sinks EventSinkList
	_, err := c.httpClient.Get(ctx, "/api/event-sinks", &sinks)
	return &sinks, err
}

func (c *ApiClient) GetEventSink(ctx context.Context, id string) (*EventSinkDetails, error) {
	var sink EventSinkDetails
	_, err := c.httpClient.Get(ctx, "/api/event-sinks/"+id, &sink)
	return &sink, err
}

func (c *ApiClient) CreateEventSink(ctx context.Context, req EventSinkCreateRequest) (*EventSinkCreateResponse, error) {
	var resp EventSinkCreateResponse
	_, err := c.httpClient.Post(ctx, "/api/event-sinks", req, &resp, 201)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *ApiClient) UpdateEventSink(ctx context.Context, sinkId string, req EventSinkUpdateRequest) error {
	_, err := c.httpClient.Put(ctx, "/api/event-sinks/"+sinkId, req, nil, 200)
	return err
}

func (c *ApiClient) DeleteEventSink(ctx context.Context, id string) error {
	_, err := c.httpClient.Delete(ctx, "/api/event-sinks/"+id, nil, nil, 200)
	return err
}

func (c *ApiClient) EmitEvent(ctx context.Context, spaceId string, req EmitEventRequest) error {
	_, err := c.httpClient.Post(ctx, "/api/spaces/"+spaceId+"/emit-event", req, nil, 200)
	return err
}
