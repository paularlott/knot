package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/template"

	"github.com/paularlott/gossip/hlc"
)

type EventRenderData struct {
	EventId      string
	EventType    string
	SpaceId      string
	SpaceName    string
	UserId       string
	Username     string
	PoolName     string
	PortURLs     map[string]string
	CustomFields map[string]string
	Payload      map[string]interface{}
	Ts           hlc.Timestamp
	ActorId      string
	ActorName    string
	ActorKind    string
}

const DefaultWebhookBodyTemplate = `{
  "event_id":   "${{ .event.id }}",
  "event_type": "${{ .event.type }}",
  "event_ts":   "${{ .event.ts }}",
  "data": ${{ json .event.data }}
}`

func RenderEventTemplate(bodyTemplate string, data *EventRenderData) ([]byte, error) {
	if bodyTemplate == "" {
		bodyTemplate = DefaultWebhookBodyTemplate
	}

	funcs := map[string]any{
		"map": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, errors.New("map requires key value pairs")
			}
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, fmt.Errorf("type %T is not usable as map key", pairs[i])
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},
		"quote": func(s string) string {
			return strings.ReplaceAll(s, `"`, `\"`)
		},
		"toUpper": strings.ToUpper,
		"toLower": strings.ToLower,
		"json": func(v interface{}) string {
			b, _ := json.Marshal(v)
			return string(b)
		},
	}

	tmpl, err := template.New("event").Funcs(funcs).Delims("${{", "}}").Parse(bodyTemplate)
	if err != nil {
		return nil, err
	}

	if data.PortURLs == nil {
		data.PortURLs = map[string]string{}
	}
	if data.CustomFields == nil {
		data.CustomFields = map[string]string{}
	}

	var tsStr string
	if !data.Ts.Equal(hlc.Timestamp(0)) {
		tsStr = data.Ts.Time().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	}

	renderData := map[string]interface{}{
		"event": map[string]interface{}{
			"id":   data.EventId,
			"type": data.EventType,
			"ts":   tsStr,
			"data": data.Payload,
		},
		"space": map[string]interface{}{
			"id":   data.SpaceId,
			"name": data.SpaceName,
			"urls": data.PortURLs,
		},
		"actor": map[string]interface{}{
			"id":       data.ActorId,
			"username": data.ActorName,
			"kind":     data.ActorKind,
		},
		"custom": data.CustomFields,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, renderData); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
