package msg

import (
	"encoding/json"

	"github.com/paularlott/knot/internal/methods"
)

type RegisterMethodsRequest struct {
	Registration methods.Registration `json:"registration" msgpack:"registration"`
}

type RegisterMethodsResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}

type CallMethodRequest struct {
	Method        string          `json:"method" msgpack:"method"`
	Params        json.RawMessage `json:"params,omitempty" msgpack:"params,omitempty"`
	ID            any             `json:"id,omitempty" msgpack:"id,omitempty"`
	IsNotification bool           `json:"-" msgpack:"is_notification,omitempty"`
}

type CallMethodResponse struct {
	Response methods.JSONRPCResponse `json:"response" msgpack:"response"`
}
