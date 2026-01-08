package apiclient

type ScriptList struct {
	Count   int          `json:"count"`
	Scripts []ScriptInfo `json:"scripts"`
}

type ScriptInfo struct {
	Id          string   `json:"script_id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Groups      []string `json:"groups"`
	Active      bool     `json:"active"`
	ScriptType  string   `json:"script_type"`
	Timeout     int      `json:"timeout"`
}

type ScriptDetails struct {
	Id                 string   `json:"script_id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Content            string   `json:"content"`
	Groups             []string `json:"groups"`
	Active             bool     `json:"active"`
	ScriptType         string   `json:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords"`
	Timeout            int      `json:"timeout"`
}

type ScriptCreateRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Content            string   `json:"content"`
	Groups             []string `json:"groups"`
	Active             bool     `json:"active"`
	ScriptType         string   `json:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords"`
	Timeout            int      `json:"timeout"`
}

type ScriptUpdateRequest struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Content            string   `json:"content"`
	Groups             []string `json:"groups"`
	Active             bool     `json:"active"`
	ScriptType         string   `json:"script_type"`
	MCPInputSchemaToml string   `json:"mcp_input_schema_toml"`
	MCPKeywords        []string `json:"mcp_keywords"`
	Timeout            int      `json:"timeout"`
}

type ScriptCreateResponse struct {
	Status bool   `json:"status"`
	Id     string `json:"script_id"`
}

type ScriptExecuteRequest struct {
	Arguments []string `json:"arguments"`
}

type ScriptExecuteResponse struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

type ScriptContentExecuteRequest struct {
	Content   string   `json:"content"`
	Arguments []string `json:"arguments"`
}

type ScriptNameExecuteRequest struct {
	ScriptName string   `json:"script_name"`
	Arguments  []string `json:"arguments"`
}

type ScriptLibraryResponse struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}
