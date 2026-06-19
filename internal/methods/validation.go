package methods

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

var methodNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

var reservedPrefixes = []string{
	"rpc.",
	"knot.",
	"space.",
	"user.",
}

func LoadTOMLFile(path string, spaceName string) (*Registration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadTOML(data, spaceName)
}

func LoadRawTOMLFile(path string) (*Registration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return LoadRawTOML(data)
}

// LoadRawTOML decodes TOML bytes into a Registration without running
// normalization or validation. Callers (typically the agent) forward the
// decoded struct to the knot server, which validates it during registration.
func LoadRawTOML(data []byte) (*Registration, error) {
	var reg Registration
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&reg); err != nil {
		return nil, err
	}
	return &reg, nil
}

func LoadTOML(data []byte, spaceName string) (*Registration, error) {
	var reg Registration
	if _, err := toml.NewDecoder(bytes.NewReader(data)).Decode(&reg); err != nil {
		return nil, err
	}
	if err := NormalizeAndValidate(&reg, spaceName); err != nil {
		return nil, err
	}
	return &reg, nil
}

func NormalizeAndValidate(reg *Registration, spaceName string) error {
	if reg == nil {
		return fmt.Errorf("registration is nil")
	}
	if reg.Server.Type == "" {
		reg.Server.Type = ServerTypeStdio
	}
	if reg.Server.Type != ServerTypeStdio {
		return fmt.Errorf("unsupported server type %q", reg.Server.Type)
	}
	if strings.TrimSpace(reg.Server.Command) == "" {
		return fmt.Errorf("server command is required")
	}
	if reg.Server.Timeout == 0 {
		reg.Server.Timeout = 30
	}
	if reg.Server.Timeout < 0 {
		return fmt.Errorf("server timeout must be positive")
	}
	reg.Server.Mode = strings.TrimSpace(reg.Server.Mode)
	if reg.Server.Mode == "" {
		reg.Server.Mode = ModeConcurrent
	}
	if reg.Server.Mode != ModeConcurrent && reg.Server.Mode != ModeSerial {
		return fmt.Errorf("server mode must be %q or %q", ModeConcurrent, ModeSerial)
	}
	if len(reg.Methods) == 0 {
		return fmt.Errorf("at least one method is required")
	}

	seen := map[string]bool{}
	seenMCPTools := map[string]string{}
	for i := range reg.Methods {
		method := &reg.Methods[i]
		method.Name = strings.ReplaceAll(method.Name, "{{space}}", spaceName)
		method.Name = strings.TrimSpace(method.Name)
		method.LocalName = strings.TrimSpace(method.LocalName)
		method.Scope = strings.TrimSpace(method.Scope)

		if method.Name == "" {
			return fmt.Errorf("method %d name is required", i+1)
		}
		if !methodNameRe.MatchString(method.Name) {
			return fmt.Errorf("method %q has invalid name", method.Name)
		}
		for _, prefix := range reservedPrefixes {
			if strings.HasPrefix(method.Name, prefix) || method.Name == strings.TrimSuffix(prefix, ".") {
				return fmt.Errorf("method %q uses reserved prefix %q", method.Name, strings.TrimSuffix(prefix, "."))
			}
		}
		if seen[method.Name] {
			return fmt.Errorf("duplicate method %q in registration", method.Name)
		}
		seen[method.Name] = true

		if method.LocalName == "" {
			method.LocalName = method.Name
		}
		if !methodNameRe.MatchString(method.LocalName) {
			return fmt.Errorf("method %q has invalid local_name %q", method.Name, method.LocalName)
		}
		if method.Scope == "" {
			method.Scope = ScopePrivate
		}
		if method.Scope != ScopePrivate && method.Scope != ScopeShared {
			return fmt.Errorf("method %q has invalid scope %q", method.Name, method.Scope)
		}
		if method.Description == "" {
			return fmt.Errorf("method %q description is required", method.Name)
		}
		if method.ParamsSchema != nil {
			if err := validateSchema(method.Name, "params_schema", method.ParamsSchema); err != nil {
				return err
			}
		}
		if method.ResultSchema != nil {
			if err := validateSchema(method.Name, "result_schema", method.ResultSchema); err != nil {
				return err
			}
		}
		if method.MCPTool {
			toolName := MCPToolName(method.Name)
			if existing, ok := seenMCPTools[toolName]; ok {
				return fmt.Errorf("methods %q and %q produce the same MCP tool name %q", existing, method.Name, toolName)
			}
			seenMCPTools[toolName] = method.Name
		}
	}

	return nil
}

func validateSchema(methodName string, field string, schema map[string]any) error {
	if len(schema) == 0 {
		return nil
	}
	if typ, ok := schema["type"]; ok {
		switch typ {
		case "object", "array", "string", "integer", "number", "boolean", "null":
		default:
			return fmt.Errorf("method %q %s has unsupported type %v", methodName, field, typ)
		}
	}
	if _, err := json.Marshal(schema); err != nil {
		return fmt.Errorf("method %q %s is not JSON serializable: %w", methodName, field, err)
	}
	return nil
}

func MCPToolName(methodName string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range methodName {
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_'
		if r == '.' || !valid {
			if !lastUnderscore {
				b.WriteRune('_')
				lastUnderscore = true
			}
			continue
		}
		b.WriteRune(r)
		lastUnderscore = false
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "method"
	}
	if name[0] >= '0' && name[0] <= '9' {
		name = "method_" + name
	}
	return name
}
