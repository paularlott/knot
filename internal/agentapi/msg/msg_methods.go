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

// CallMethodBatchItem is one call inside a batch sent from the knot server to
// the agent. The server has already routed and rewritten Method to local_name.
type CallMethodBatchItem struct {
	Method         string          `json:"method" msgpack:"method"`
	Params         json.RawMessage `json:"params,omitempty" msgpack:"params,omitempty"`
	ID             any             `json:"id,omitempty" msgpack:"id,omitempty"`
	IsNotification bool            `json:"-" msgpack:"is_notification,omitempty"`
}

// CallMethodBatchRequest carries a sub-batch of calls targeting the same
// agent/space. The server groups incoming batch items by destination space
// and sends each group as one CallMethodBatchRequest over a single yamux
// stream.
type CallMethodBatchRequest struct {
	Items []CallMethodBatchItem `json:"items" msgpack:"items"`
}

// CallMethodBatchResponse is the agent's reply to a batch. Responses are in
// the same order as the request items that had IDs. Notifications produce no
// response entry.
type CallMethodBatchResponse struct {
	Responses []methods.JSONRPCResponse `json:"responses" msgpack:"responses"`
}
