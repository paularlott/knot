package methods

import "testing"

func TestLoadTOMLExpandsSpaceAndDefaults(t *testing.T) {
	reg, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"

[[methods]]
name = "{{space}}.search"
local_name = "search"
description = "Search things"
`), "notes")
	if err != nil {
		t.Fatalf("LoadTOML() error = %v", err)
	}
	if reg.Server.Timeout != 30 {
		t.Fatalf("expected default timeout 30, got %d", reg.Server.Timeout)
	}
	if reg.Methods[0].Name != "notes.search" {
		t.Fatalf("expected expanded method name, got %q", reg.Methods[0].Name)
	}
	if reg.Methods[0].Scope != ScopePrivate {
		t.Fatalf("expected default private scope, got %q", reg.Methods[0].Scope)
	}
	if reg.Methods[0].MCPTool {
		t.Fatalf("expected mcp_tool to default false")
	}
}

func TestLoadTOMLRejectsReservedPrefix(t *testing.T) {
	_, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"

[[methods]]
name = "rpc.test"
description = "Reserved"
`), "notes")
	if err == nil {
		t.Fatalf("expected reserved prefix error")
	}
}

func TestLoadTOMLRejectsDuplicateMCPToolName(t *testing.T) {
	_, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"

[[methods]]
name = "notes.search"
description = "Search notes"
mcp_tool = true

[[methods]]
name = "notes-search"
description = "Search notes another way"
mcp_tool = true
`), "notes")
	if err == nil {
		t.Fatalf("expected duplicate MCP tool name error")
	}
}

func TestMCPToolName(t *testing.T) {
	got := MCPToolName("notes.search")
	if got != "notes_search" {
		t.Fatalf("expected notes_search, got %q", got)
	}
	got = MCPToolName("123.bad-name")
	if got != "method_123_bad_name" {
		t.Fatalf("expected method_123_bad_name, got %q", got)
	}
}

func TestLoadTOMLDefaultsModeToConcurrent(t *testing.T) {
	reg, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"

[[methods]]
name = "search"
description = "Search things"
`), "notes")
	if err != nil {
		t.Fatalf("LoadTOML() error = %v", err)
	}
	if reg.Server.Mode != ModeConcurrent {
		t.Fatalf("expected default mode %q, got %q", ModeConcurrent, reg.Server.Mode)
	}
}

func TestLoadTOMLAcceptsSerialMode(t *testing.T) {
	reg, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"
mode = "serial"

[[methods]]
name = "search"
description = "Search things"
`), "notes")
	if err != nil {
		t.Fatalf("LoadTOML() error = %v", err)
	}
	if reg.Server.Mode != ModeSerial {
		t.Fatalf("expected mode %q, got %q", ModeSerial, reg.Server.Mode)
	}
}

func TestLoadTOMLRejectsUnknownMode(t *testing.T) {
	_, err := LoadTOML([]byte(`
[server]
type = "stdio"
command = "./bin/method-server"
mode = "pipelined"

[[methods]]
name = "search"
description = "Search things"
`), "notes")
	if err == nil {
		t.Fatalf("expected unknown mode error")
	}
}
