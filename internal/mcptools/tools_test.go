package mcptools

import (
	"testing"

	"github.com/paularlott/mcp/toolmetadata"
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
