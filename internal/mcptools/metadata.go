package mcptools

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/paularlott/lmchatkit"
	"github.com/paularlott/mcp"
	"github.com/paularlott/mcp/toolmetadata"
)

func ParseMetadata(tomlContent []byte) (*toolmetadata.ToolMetadata, error) {
	var metadata toolmetadata.ToolMetadata
	if err := toml.Unmarshal(tomlContent, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse TOML: %w", err)
	}

	if metadata.Description == "" {
		return nil, fmt.Errorf("tool description is required")
	}

	// requires_approval and always_confirm are published as destructiveHint,
	// so MCP clients ask the user before the tool runs. An explicit
	// destructive key wins.
	if metadata.Destructive == nil && (ParseRequiresApproval(tomlContent) || ParseAlwaysConfirm(tomlContent) != nil) {
		destructive := true
		metadata.Destructive = &destructive
	}

	return &metadata, nil
}

// ParseAlwaysConfirm returns a tool's always_confirm setting as the value of
// its lmchatkit.AlwaysConfirmMetaKey _meta entry: true when every call must
// be confirmed, or a map of the argument values that pick the calls that
// must (always_confirm = { mode = "write" }). The web assistant asks for
// those calls every time and offers no "Always Allow". Nil when unset.
func ParseAlwaysConfirm(tomlContent []byte) any {
	var v struct {
		AlwaysConfirm any `toml:"always_confirm"`
	}
	if err := toml.Unmarshal(tomlContent, &v); err != nil {
		return nil
	}
	switch c := v.AlwaysConfirm.(type) {
	case bool:
		if c {
			return true
		}
	case map[string]any:
		if len(c) == 0 {
			return true
		}
		return c
	}
	return nil
}

// ToolHints returns the annotations and _meta a user script tool's TOML asks
// for: requires_approval and always_confirm give destructiveHint, and
// always_confirm also the lmchatkit always-confirm rule. Both nil when the
// TOML asks for neither.
func ToolHints(tomlContent string) (*mcp.ToolAnnotations, map[string]any) {
	if tomlContent == "" {
		return nil, nil
	}
	confirm := ParseAlwaysConfirm([]byte(tomlContent))
	if confirm == nil && !ParseRequiresApproval([]byte(tomlContent)) {
		return nil, nil
	}
	destructive := true
	annotations := &mcp.ToolAnnotations{DestructiveHint: &destructive}
	if confirm == nil {
		return annotations, nil
	}
	return annotations, map[string]any{lmchatkit.AlwaysConfirmMetaKey: confirm}
}

// ParseRequiresApproval reports whether a tool's TOML sets requires_approval
// (or its older name requires_permission).
func ParseRequiresApproval(tomlContent []byte) bool {
	var v struct {
		RequiresApproval   bool `toml:"requires_approval"`
		RequiresPermission bool `toml:"requires_permission"`
	}
	if err := toml.Unmarshal(tomlContent, &v); err != nil {
		return false
	}
	return v.RequiresApproval || v.RequiresPermission
}

// ParseHandler returns the tool's handler kind: "go" for a tool implemented
// in Go (go_tools.go), "" for a scripted tool (the .py beside the TOML).
func ParseHandler(tomlContent []byte) string {
	var v struct {
		Handler string `toml:"handler"`
	}
	if err := toml.Unmarshal(tomlContent, &v); err != nil {
		return ""
	}
	return v.Handler
}
