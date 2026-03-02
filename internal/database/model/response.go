package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/log"
)

// ResponseStatus represents the status of a response
type ResponseStatus string

const (
	StatusPending    ResponseStatus = "pending"
	StatusInProgress ResponseStatus = "in_progress"
	StatusCompleted  ResponseStatus = "completed"
	StatusCancelled  ResponseStatus = "cancelled"
	StatusFailed     ResponseStatus = "failed"
)

// Response object for OpenAI Responses API
type Response struct {
	Id                string                 `json:"response_id" db:"response_id,pk" msgpack:"response_id"`
	Status            ResponseStatus         `json:"status" db:"status" msgpack:"status"`
	Request           map[string]interface{} `json:"request" db:"request,json" msgpack:"request"`
	Response          map[string]interface{} `json:"response" db:"response,json" msgpack:"response"`
	Error             string                 `json:"error,omitempty" db:"error_text" msgpack:"error"`
	PreviousResponseId string                `json:"previous_response_id,omitempty" db:"previous_response_id" msgpack:"previous_response_id"`
	UserId            string                 `json:"user_id" db:"user_id" msgpack:"user_id"`
	SpaceId           string                 `json:"space_id,omitempty" db:"space_id" msgpack:"space_id"`
	ExpiresAt         *time.Time             `json:"expires_at,omitempty" db:"expires_at" msgpack:"expires_at"`
	IsDeleted         bool                   `json:"is_deleted" db:"is_deleted" msgpack:"is_deleted"`
	CreatedAt         time.Time              `json:"created_at" db:"created_at" msgpack:"created_at"`
	UpdatedAt         hlc.Timestamp          `json:"updated_at" db:"updated_at" msgpack:"updated_at"`
}

// NewResponse creates a new Response object
func NewResponse(userId string, spaceId string, ttl time.Duration) *Response {
	id, err := uuid.NewV7()
	if err != nil {
		log.Fatal(err.Error())
	}

	var expiresAt *time.Time
	if ttl > 0 {
		exp := time.Now().UTC().Add(ttl)
		expiresAt = &exp
	}

	response := &Response{
		Id:        id.String(),
		Status:    StatusPending,
		Request:   make(map[string]interface{}),
		Response:  make(map[string]interface{}),
		UserId:    userId,
		SpaceId:   spaceId,
		ExpiresAt: expiresAt,
		IsDeleted: false,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: hlc.Now(),
	}

	return response
}

// SetRequest sets the request data from a JSON-serializable structure
func (r *Response) SetRequest(data interface{}) error {
	if r.Request == nil {
		r.Request = make(map[string]interface{})
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, &r.Request)
}

// GetRequest populates the request data into a structure
func (r *Response) GetRequest(data interface{}) error {
	bytes, err := json.Marshal(r.Request)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, data)
}

// SetResponse sets the response data from a JSON-serializable structure
func (r *Response) SetResponse(data interface{}) error {
	if r.Response == nil {
		r.Response = make(map[string]interface{})
	}
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, &r.Response)
}

// GetResponse populates the response data into a structure
func (r *Response) GetResponse(data interface{}) error {
	bytes, err := json.Marshal(r.Response)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, data)
}
