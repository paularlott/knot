package apiclient

type ScriptList struct {
	Count   int          `json:"count" msgpack:"count"`
	Scripts []ScriptInfo `json:"scripts" msgpack:"scripts"`
}

type ScriptInfo struct {
	Id           string   `json:"script_id" msgpack:"script_id"`
	UserId       string   `json:"user_id" msgpack:"user_id"`
	Name         string   `json:"name" msgpack:"name"`
	Description  string   `json:"description" msgpack:"description"`
	Groups       []string `json:"groups" msgpack:"groups"`
	Zones        []string `json:"zones" msgpack:"zones"`
	Active       bool     `json:"active" msgpack:"active"`
	ScriptType   string   `json:"script_type" msgpack:"script_type"`
	Discoverable bool     `json:"discoverable" msgpack:"discoverable"`
	IsManaged    bool     `json:"is_managed" msgpack:"is_managed"`
}

type ScriptDetails struct {
	Id                 string   `json:"script_id" msgpack:"script_id"`
	UserId             string   `json:"user_id" msgpack:"user_id"`
	Name               string   `json:"name" msgpack:"name"`
	Description        string   `json:"description" msgpack:"description"`
	Content            string   `json:"content" msgpack:"content"`
	Groups             []string `json:"groups" msgpack:"groups"`
	Zones              []string `json:"zones" msgpack:"zones"`
	Active             bool     `json:"active" msgpack:"active"`
	ScriptType         string   `json:"script_type" msgpack:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml" msgpack:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords" msgpack:"mcp_keywords"`
	Discoverable       bool     `json:"discoverable" msgpack:"discoverable"`
	IsManaged          bool     `json:"is_managed" msgpack:"is_managed"`
}

type ScriptCreateRequest struct {
	UserId             string   `json:"user_id" msgpack:"user_id"`
	Name               string   `json:"name" msgpack:"name"`
	Description        string   `json:"description" msgpack:"description"`
	Content            string   `json:"content" msgpack:"content"`
	Groups             []string `json:"groups" msgpack:"groups"`
	Zones              []string `json:"zones" msgpack:"zones"`
	Active             bool     `json:"active" msgpack:"active"`
	ScriptType         string   `json:"script_type" msgpack:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml" msgpack:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords" msgpack:"mcp_keywords"`
	Discoverable       bool     `json:"discoverable" msgpack:"discoverable"`
}

type ScriptUpdateRequest struct {
	Name               string   `json:"name" msgpack:"name"`
	Description        string   `json:"description" msgpack:"description"`
	Content            string   `json:"content" msgpack:"content"`
	Groups             []string `json:"groups" msgpack:"groups"`
	Zones              []string `json:"zones" msgpack:"zones"`
	Active             bool     `json:"active" msgpack:"active"`
	ScriptType         string   `json:"script_type" msgpack:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml" msgpack:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords" msgpack:"mcp_keywords"`
	Discoverable       bool     `json:"discoverable" msgpack:"discoverable"`
}

type ScriptCreateResponse struct {
	Status bool   `json:"status" msgpack:"status"`
	Id     string `json:"script_id" msgpack:"script_id"`
}

type ScriptExecuteRequest struct {
	Arguments []string `json:"arguments" msgpack:"arguments"`
}

type ScriptExecuteResponse struct {
	Output   string `json:"output" msgpack:"output"`
	Error    string `json:"error,omitempty" msgpack:"error,omitempty"`
	ExitCode int    `json:"exit_code" msgpack:"exit_code"`
}

type ScriptContentExecuteRequest struct {
	Content   string   `json:"content" msgpack:"content"`
	Arguments []string `json:"arguments" msgpack:"arguments"`
}

type ScriptNameExecuteRequest struct {
	ScriptName string   `json:"script_name" msgpack:"script_name"`
	Arguments  []string `json:"arguments" msgpack:"arguments"`
}

type UnifiedScriptExecuteRequest struct {
	ScriptId   string   `json:"script_id,omitempty" msgpack:"script_id,omitempty"`
	ScriptName string   `json:"script_name,omitempty" msgpack:"script_name,omitempty"`
	Content    string   `json:"content,omitempty" msgpack:"content,omitempty"`
	Arguments  []string `json:"arguments" msgpack:"arguments"`
}

type ScriptLibraryResponse struct {
	Name    string `json:"name" msgpack:"name"`
	Content string `json:"content" msgpack:"content"`
}
