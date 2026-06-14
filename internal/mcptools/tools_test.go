package mcptools

import (
	"regexp"
	"testing"

	"github.com/paularlott/mcp/toolmetadata"
)

// validParamTypes is the set of type strings supported by internal/mcp/toml_schema.go.
var validParamTypes = map[string]bool{
	"string":          true,
	"int":             true,
	"integer":         true,
	"float":           true,
	"number":          true,
	"bool":            true,
	"boolean":         true,
	"array:string":    true,
	"array:number":    true,
	"array:int":       true,
	"array:integer":   true,
	"array:float":     true,
	"array:bool":      true,
	"array:boolean":   true,
}

var (
	toolNamePattern   = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	paramNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	returnCallPattern = regexp.MustCompile(`tool\.return_(string|object|toon|error)\s*\(`)
	importPattern     = regexp.MustCompile(`(?m)^import\s+scriptling\.mcp\.tool\b`)
)

func TestCreateSpaceToolIncludesCustomFields(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools() failed: %v", err)
	}

	tool, ok := GetTool("create_space")
	if !ok {
		t.Fatal("create_space tool not loaded")
	}

	builder, err := toolmetadata.BuildMCPTool(tool.Name, tool.Metadata)
	if err != nil {
		t.Fatalf("BuildMCPTool(create_space) failed: %v", err)
	}

	schema := builder.BuildSchema()
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema missing properties map: %+v", schema)
	}

	customFields, ok := props["custom_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema missing custom_fields: %+v", props)
	}
	if customFields["type"] != "array" {
		t.Fatalf("custom_fields type = %v, want array", customFields["type"])
	}
	items, ok := customFields["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("custom_fields missing items schema: %+v", customFields)
	}
	if items["type"] != "string" {
		t.Fatalf("custom_fields item type = %v, want string", items["type"])
	}
}

func TestUpdateSpaceToolIncludesCustomFields(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools() failed: %v", err)
	}

	tool, ok := GetTool("update_space")
	if !ok {
		t.Fatal("update_space tool not loaded")
	}

	builder, err := toolmetadata.BuildMCPTool(tool.Name, tool.Metadata)
	if err != nil {
		t.Fatalf("BuildMCPTool(update_space) failed: %v", err)
	}

	schema := builder.BuildSchema()
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema missing properties map: %+v", schema)
	}

	customFields, ok := props["custom_fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("schema missing custom_fields: %+v", props)
	}
	if customFields["type"] != "array" {
		t.Fatalf("custom_fields type = %v, want array", customFields["type"])
	}
	items, ok := customFields["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("custom_fields missing items schema: %+v", customFields)
	}
	if items["type"] != "string" {
		t.Fatalf("custom_fields item type = %v, want string", items["type"])
	}

	newName, ok := props["new_name"]
	if !ok {
		t.Fatalf("schema missing new_name: %+v", props)
	}
	_ = newName
}

// TestAllToolsConformToSchema sweeps every loaded tool and asserts a set of
// structural invariants. Cheap to run, catches a wide class of mistakes
// (missing fields, invalid types, duplicate params, broken python) in one go.
func TestAllToolsConformToSchema(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools() failed: %v", err)
	}

	tools := ListTools()
	if len(tools) == 0 {
		t.Fatal("no tools loaded")
	}

	seenDescriptions := make(map[string]string, len(tools)) // description -> tool name

	for _, tool := range tools {
		t.Run(tool.Name, func(t *testing.T) {
			// Tool name
			if !toolNamePattern.MatchString(tool.Name) {
				t.Errorf("tool name %q does not match [a-z][a-z0-9_]*", tool.Name)
			}

			// Description
			desc := tool.Metadata.Description
			if desc == "" {
				t.Errorf("description is empty")
			}
			if len(desc) > 300 {
				t.Errorf("description length %d exceeds 300 chars (keep tool list scannable)", len(desc))
			}
			if prev, ok := seenDescriptions[desc]; ok {
				t.Errorf("description is identical to tool %q (descriptions should be unique to aid disambiguation)", prev)
			}
			seenDescriptions[desc] = tool.Name

			// Keywords (convention: every discoverable tool should have at least one)
			if len(tool.Metadata.Keywords) == 0 {
				t.Errorf("no keywords (tools should expose search terms)")
			}
			seenKw := make(map[string]bool, len(tool.Metadata.Keywords))
			for _, kw := range tool.Metadata.Keywords {
				if kw == "" {
					t.Errorf("empty keyword in %s", tool.Name)
					continue
				}
				if seenKw[kw] {
					t.Errorf("duplicate keyword %q in %s", kw, tool.Name)
				}
				seenKw[kw] = true
			}

			// Parameters
			seenParams := make(map[string]bool, len(tool.Metadata.Parameters))
			for i, p := range tool.Metadata.Parameters {
				if p.Name == "" {
					t.Errorf("parameters[%d].name is empty", i)
					continue
				}
				if !paramNamePattern.MatchString(p.Name) {
					t.Errorf("parameter %q name does not match [a-z][a-z0-9_]*", p.Name)
				}
				if seenParams[p.Name] {
					t.Errorf("duplicate parameter %q", p.Name)
				}
				seenParams[p.Name] = true

				if p.Type == "" {
					t.Errorf("parameter %q has empty type", p.Name)
				} else if !validParamTypes[p.Type] {
					t.Errorf("parameter %q has unsupported type %q (will silently coerce to string)", p.Name, p.Type)
				}

				if p.Description == "" {
					t.Errorf("parameter %q has empty description", p.Name)
				}
			}

			// Python script sanity
			script := tool.Script
			if script == "" {
				t.Fatalf("script body is empty")
			}
			if !importPattern.MatchString(script) {
				t.Errorf("script does not import scriptling.mcp.tool")
			}
			if !returnCallPattern.MatchString(script) {
				t.Errorf("script has no tool.return_* call (every tool must terminate by returning a value)")
			}

			// Required parameters should be read by the script.
			// Catches the case where a param is marked required in TOML but never consumed.
			for _, p := range tool.Metadata.Parameters {
				if !p.Required {
					continue
				}
				// Look for tool.get_*("<name>" ...). Allow any getter.
				getPattern := regexp.MustCompile(`tool\.get_(string|int|float|bool|list|string_list|int_list|float_list|bool_list)\s*\(\s*"` + regexp.QuoteMeta(p.Name) + `"`)
				if !getPattern.MatchString(script) {
					t.Errorf("required parameter %q is never read by the script (expected tool.get_*(%q, ...))", p.Name, p.Name)
				}
			}
		})
	}
}

// TestAllToolsBuildSchema verifies each tool's metadata can be converted into
// a valid MCP JSON schema without error. Catches malformed parameter combos.
func TestAllToolsBuildSchema(t *testing.T) {
	if err := LoadTools("", nil); err != nil {
		t.Fatalf("LoadTools() failed: %v", err)
	}

	for _, tool := range ListTools() {
		t.Run(tool.Name, func(t *testing.T) {
			builder, err := toolmetadata.BuildMCPTool(tool.Name, tool.Metadata)
			if err != nil {
				t.Fatalf("BuildMCPTool failed: %v", err)
			}
			schema := builder.BuildSchema()
			if schema == nil {
				t.Fatalf("BuildSchema returned nil")
			}
			props, ok := schema["properties"].(map[string]interface{})
			if !ok && len(tool.Metadata.Parameters) > 0 {
				t.Fatalf("schema missing properties map despite %d declared params", len(tool.Metadata.Parameters))
			}
			for _, p := range tool.Metadata.Parameters {
				if _, ok := props[p.Name].(map[string]interface{}); !ok {
					t.Errorf("declared parameter %q missing from built schema properties", p.Name)
				}
			}
		})
	}
}
