package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

type EventSink struct {
	Id            string         `json:"event_sink_id" db:"event_sink_id,pk" msgpack:"event_sink_id"`
	UserId        string         `json:"user_id" db:"user_id" msgpack:"user_id"`
	Name          string         `json:"name" db:"name" msgpack:"name"`
	Description   string         `json:"description" db:"description" msgpack:"description"`
	Events        []string       `json:"events" db:"events,json" msgpack:"events"`
	SinkType      string         `json:"sink_type" db:"sink_type" msgpack:"sink_type"`
	Webhook       *WebhookConfig `json:"webhook,omitempty" db:"webhook,json" msgpack:"webhook,omitempty"`
	ScriptId      string         `json:"script_id,omitempty" db:"script_id" msgpack:"script_id,omitempty"`
	Active        bool           `json:"active" db:"active" msgpack:"active"`
	CreatedUserId string         `json:"created_user_id" db:"created_user_id" msgpack:"created_user_id"`
	CreatedAt     time.Time      `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedUserId string         `json:"updated_user_id" db:"updated_user_id" msgpack:"updated_user_id"`
	UpdatedAt     hlc.Timestamp  `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
	IsDeleted     bool           `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
}

type WebhookConfig struct {
	URL           string            `json:"url" msgpack:"url"`
	Secret        string            `json:"secret" msgpack:"secret"`
	Headers       map[string]string `json:"headers,omitempty" msgpack:"headers,omitempty"`
	BodyTemplate  string            `json:"body_template" msgpack:"body_template"`
	SkipTLSVerify bool              `json:"skip_tls_verify" msgpack:"skip_tls_verify"`
}

func NewEventSink(name, description string, events []string, sinkType string, webhook *WebhookConfig, scriptId string, active bool, ownerUserId, createdUserId string) *EventSink {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	if sinkType == "" {
		sinkType = "webhook"
	}

	return &EventSink{
		Id:            id.String(),
		UserId:        ownerUserId,
		Name:          name,
		Description:   description,
		Events:        events,
		SinkType:      sinkType,
		Webhook:       webhook,
		ScriptId:      scriptId,
		Active:        active,
		CreatedUserId: createdUserId,
		CreatedAt:     time.Now().UTC(),
		UpdatedUserId: createdUserId,
		UpdatedAt:     hlc.Now(),
	}
}

func (s *EventSink) IsGlobalSink() bool {
	return s.UserId == ""
}

// MatchEventType tests if an event type matches any of the sink's patterns.
func (s *EventSink) MatchEventType(eventType string) bool {
	for _, pattern := range s.Events {
		if pattern == "*" {
			return true
		}
		if len(pattern) > 2 && pattern[len(pattern)-2:] == ".*" {
			prefix := pattern[:len(pattern)-1]
			if len(eventType) >= len(prefix) && eventType[:len(prefix)] == prefix {
				return true
			}
			continue
		}
		if pattern == eventType {
			return true
		}
	}
	return false
}
