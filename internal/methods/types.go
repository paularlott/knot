package methods

import "encoding/json"

const (
	ScopePrivate = "private"
	ScopeShared  = "shared"

	ServerTypeStdio = "stdio"

	// ModeConcurrent allows multiple JSON-RPC requests to be in flight on the
	// method server at the same time. Responses are correlated by id. This is
	// the default.
	ModeConcurrent = "concurrent"
	// ModeSerial lets only one JSON-RPC request be in flight on the method
	// server at a time. Useful for method servers that are not re-entrant.
	ModeSerial = "serial"
)

type ServerConfig struct {
	Type    string   `json:"type" toml:"type" msgpack:"type"`
	Command string   `json:"command" toml:"command" msgpack:"command"`
	Args    []string `json:"args,omitempty" toml:"args" msgpack:"args,omitempty"`
	Timeout int      `json:"timeout,omitempty" toml:"timeout" msgpack:"timeout,omitempty"`
	Mode    string   `json:"mode,omitempty" toml:"mode" msgpack:"mode,omitempty"`
}

type MethodDefinition struct {
	Name         string         `json:"name" toml:"name" msgpack:"name"`
	LocalName    string         `json:"local_name" toml:"local_name" msgpack:"local_name"`
	Description  string         `json:"description" toml:"description" msgpack:"description"`
	Keywords     []string       `json:"keywords,omitempty" toml:"keywords" msgpack:"keywords,omitempty"`
	Scope        string         `json:"scope,omitempty" toml:"scope" msgpack:"scope,omitempty"`
	Groups       []string       `json:"groups,omitempty" toml:"groups" msgpack:"groups,omitempty"`
	MCPTool      bool           `json:"mcp_tool" toml:"mcp_tool" msgpack:"mcp_tool"`
	Events       []string       `json:"events,omitempty" toml:"events,omitempty" msgpack:"events,omitempty"`
	EventSinks   []string       `json:"event_sinks,omitempty" toml:"event_sinks,omitempty" msgpack:"event_sinks,omitempty"`
	ParamsSchema map[string]any `json:"params_schema,omitempty" toml:"params_schema" msgpack:"params_schema,omitempty"`
	ResultSchema map[string]any `json:"result_schema,omitempty" toml:"result_schema" msgpack:"result_schema,omitempty"`
}

type Registration struct {
	Server  ServerConfig       `json:"server" toml:"server" msgpack:"server"`
	Methods []MethodDefinition `json:"methods" toml:"methods" msgpack:"methods"`
}

type MethodInfo struct {
	Name          string         `json:"name" msgpack:"name"`
	LocalName     string         `json:"local_name" msgpack:"local_name"`
	Description   string         `json:"description" msgpack:"description"`
	Keywords      []string       `json:"keywords,omitempty" msgpack:"keywords,omitempty"`
	Scope         string         `json:"scope" msgpack:"scope"`
	Groups        []string       `json:"groups,omitempty" msgpack:"groups,omitempty"`
	MCPTool       bool           `json:"mcp_tool" msgpack:"mcp_tool"`
	Events        []string       `json:"events,omitempty" msgpack:"events,omitempty"`
	EventSinks    []string       `json:"event_sinks,omitempty" msgpack:"event_sinks,omitempty"`
	ParamsSchema  map[string]any `json:"params_schema,omitempty" msgpack:"params_schema,omitempty"`
	ResultSchema  map[string]any `json:"result_schema,omitempty" msgpack:"result_schema,omitempty"`
	OwnerID       string         `json:"owner_id" msgpack:"owner_id"`
	Owner         string         `json:"owner" msgpack:"owner"`
	ProviderCount int            `json:"provider_count" msgpack:"provider_count"`
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc" msgpack:"jsonrpc"`
	Method  string          `json:"method" msgpack:"method"`
	Params  json.RawMessage `json:"params,omitempty" msgpack:"params,omitempty"`
	ID      any             `json:"id,omitempty" msgpack:"id,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc" msgpack:"jsonrpc"`
	Result  any           `json:"result,omitempty" msgpack:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty" msgpack:"error,omitempty"`
	ID      any           `json:"id,omitempty" msgpack:"id,omitempty"`
}

type JSONRPCError struct {
	Code    int    `json:"code" msgpack:"code"`
	Message string `json:"message" msgpack:"message"`
}
