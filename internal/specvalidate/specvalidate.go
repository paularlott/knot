package specvalidate

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/paularlott/knot/internal/container/nomad"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
	"gopkg.in/yaml.v3"
)

// Issue is a single validation problem. Line is the 1-based source line of the
// offending content when known (0 means the whole document / unknown).
type Issue struct {
	Field   string
	Message string
	Line    int
}

type NomadJobParser func(string) error

func ValidateTemplateSpec(platform, job, volumes string) []Issue {
	return ValidateTemplateSpecWithNomadParser(platform, job, volumes, defaultNomadJobParser)
}

func ValidateTemplateSpecWithNomadParser(platform, job, volumes string, parseNomadJob NomadJobParser) []Issue {
	switch platform {
	case model.PlatformManual:
		return nil
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		issues := validateLocalContainerJob(job)
		issues = append(issues, validateLocalVolumeDefinitions("volumes", volumes, false)...)
		return issues
	case model.PlatformNomad:
		issues := validateNomadJob(job, parseNomadJob)
		issues = append(issues, validateNomadVolumes("volumes", volumes, false)...)
		return issues
	default:
		return []Issue{{Field: "platform", Message: "unsupported platform"}}
	}
}

func ValidateVolumeSpec(platform, definition string) []Issue {
	switch platform {
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		return validateLocalVolumeDefinitions("definition", definition, true)
	case model.PlatformNomad:
		return validateNomadVolumes("definition", definition, true)
	default:
		return []Issue{{Field: "platform", Message: "unsupported platform"}}
	}
}

func defaultNomadJobParser(job string) error {
	client, err := nomad.NewClient()
	if err != nil {
		return err
	}

	_, err = client.ParseJobHCL(job)
	return err
}

func validateNomadJob(job string, parseNomadJob NomadJobParser) []Issue {
	if strings.TrimSpace(job) == "" {
		return []Issue{{Field: "job", Message: "job is required"}}
	}

	if parseNomadJob == nil {
		return []Issue{{Field: "job", Message: "no Nomad validator is configured"}}
	}

	// Resolve template variables before sending to Nomad, so ${{ .user.username }}
	// (or {% .user.username %}) etc. become placeholder values.
	// If resolution panics or fails (e.g. config not initialised), fall back to raw job.
	resolved := job
	func() {
		defer func() { recover() }()
		if r, err := model.ResolveVariables(job, &model.Template{}, nil, nil, nil); err == nil {
			resolved = r
		}
	}()

	if err := parseNomadJob(resolved); err != nil {
		return []Issue{{Field: "job", Line: lineFromError(err), Message: cleanYAMLError(err.Error())}}
	}

	return nil
}

// validateLocalContainerJob validates a local container (docker/podman/apple)
// spec by walking the decoded YAML node tree so every issue can carry the line
// of the offending content.
func validateLocalContainerJob(job string) []Issue {
	if strings.TrimSpace(job) == "" {
		return []Issue{{Field: "job", Message: "container specification is required"}}
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(job), &root); err != nil {
		return []Issue{{Field: "job", Line: lineFromError(err), Message: cleanYAMLError(err.Error())}}
	}
	mapping := docMapping(&root)
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		line := 1
		if mapping != nil {
			line = mapping.Line
		}
		return []Issue{{Field: "job", Line: line, Message: "container specification must be a YAML mapping"}}
	}

	var issues []Issue
	hasImage := false

	for i := 0; i+1 < len(mapping.Content); i += 2 {
		keyNode, valNode := mapping.Content[i], mapping.Content[i+1]
		switch keyNode.Value {
		case "image":
			hasImage = true
			if strings.TrimSpace(scalarValue(valNode)) == "" {
				issues = append(issues, Issue{Field: "job", Line: valNode.Line, Message: "image must be set"})
			}
		case "ports":
			issues = append(issues, validateStringList(valNode, "ports", validatePortMapping)...)
		case "volumes":
			issues = append(issues, validateStringList(valNode, "volumes", func(v string) error {
				return validateHostContainerPair(v, "volume")
			})...)
		case "devices":
			issues = append(issues, validateStringList(valNode, "devices", func(v string) error {
				return validateHostContainerPair(v, "device")
			})...)
		case "environment":
			issues = append(issues, validateStringList(valNode, "environment", validateEnvEntry)...)
		case "add_host":
			issues = append(issues, validateStringList(valNode, "add_host", validateAddHostEntry)...)
		case "dns":
			issues = append(issues, validateStringList(valNode, "dns", func(v string) error {
				return validateIPEntry(v, "dns")
			})...)
		case "dns_search":
			issues = append(issues, validateStringList(valNode, "dns_search", func(v string) error {
				if strings.TrimSpace(v) == "" {
					return fmt.Errorf("dns_search entry is empty")
				}
				return nil
			})...)
		case "cap_add":
			issues = append(issues, validateStringList(valNode, "cap_add", validateCapability)...)
		case "cap_drop":
			issues = append(issues, validateStringList(valNode, "cap_drop", validateCapability)...)
		case "memory":
			if v := strings.TrimSpace(scalarValue(valNode)); v != "" {
				if _, err := util.ConvertToBytes(v); err != nil {
					issues = append(issues, Issue{Field: "job", Line: valNode.Line, Message: fmt.Sprintf("invalid memory value %q (expected e.g. 512M, 4G)", v)})
				}
			}
		case "cpus":
			if v := strings.TrimSpace(scalarValue(valNode)); v != "" {
				if cpus, err := strconv.ParseFloat(v, 64); err != nil || cpus <= 0 {
					issues = append(issues, Issue{Field: "job", Line: valNode.Line, Message: fmt.Sprintf("invalid cpu value %q (expected a positive number, e.g. 1.5)", v)})
				}
			}
		case "command":
			if valNode.Kind != yaml.SequenceNode {
				issues = append(issues, Issue{Field: "job", Line: valNode.Line, Message: "command must be a list"})
			}
		case "privileged":
			if scalarValue(valNode) != "" {
				if _, err := strconv.ParseBool(scalarValue(valNode)); err != nil {
					issues = append(issues, Issue{Field: "job", Line: valNode.Line, Message: "privileged must be true or false"})
				}
			}
		case "container_name", "hostname", "network", "shell", "auth":
			// accepted, not validated
		default:
			issues = append(issues, Issue{Field: "job", Line: keyNode.Line, Message: fmt.Sprintf("unknown field %q", keyNode.Value)})
		}
	}

	if !hasImage {
		issues = append(issues, Issue{Field: "job", Line: mapping.Line, Message: "image must be set"})
	}

	return issues
}

func validateLocalVolumeDefinitions(field, volumes string, requireSingle bool) []Issue {
	if strings.TrimSpace(volumes) == "" {
		if requireSingle {
			return []Issue{{Field: field, Message: "volume definition is required"}}
		}
		return nil
	}

	var spec model.LocalStorageSpec
	if err := decodeYAMLStrict(volumes, &spec); err != nil {
		return []Issue{{Field: field, Line: lineFromError(err), Message: cleanYAMLError(err.Error())}}
	}

	if spec.Volumes == nil && len(spec.Paths) == 0 {
		return []Issue{{Field: field, Message: "definition must contain a top-level volumes map or paths list"}}
	}
	if requireSingle && len(spec.Volumes) != 1 {
		return []Issue{{Field: field, Message: "volume definition must contain exactly 1 volume"}}
	}
	if requireSingle && len(spec.Paths) > 0 {
		return []Issue{{Field: field, Message: "volume definition cannot contain paths"}}
	}
	if len(spec.Volumes) == 0 && len(spec.Paths) == 0 {
		return []Issue{{Field: field, Message: "definition must contain at least 1 volume or path"}}
	}

	return nil
}

func validateNomadVolumes(field, volumes string, requireSingle bool) []Issue {
	if strings.TrimSpace(volumes) == "" {
		if requireSingle {
			return []Issue{{Field: field, Message: "volume definition is required"}}
		}
		return nil
	}

	definitions, err := model.LoadVolumesFromYaml(volumes, nil, nil, nil, nil)
	if err != nil {
		return []Issue{{Field: field, Line: lineFromError(err), Message: cleanYAMLError(err.Error())}}
	}
	storage, err := model.LoadManagedPathsFromYaml(volumes, nil, nil, nil, nil)
	if err != nil {
		return []Issue{{Field: field, Line: lineFromError(err), Message: cleanYAMLError(err.Error())}}
	}

	if requireSingle && len(definitions.Volumes) != 1 {
		return []Issue{{Field: field, Message: "volume definition must contain exactly 1 volume"}}
	}
	if requireSingle && len(storage.Paths) > 0 {
		return []Issue{{Field: field, Message: "volume definition cannot contain paths"}}
	}
	if len(definitions.Volumes) == 0 && len(storage.Paths) == 0 {
		return []Issue{{Field: field, Message: "definition must contain at least 1 volume or path"}}
	}

	var issues []Issue
	for _, volume := range definitions.Volumes {
		if strings.TrimSpace(volume.Name) == "" {
			issues = append(issues, Issue{Field: field, Message: "volume name must be set"})
		}
		if strings.TrimSpace(volume.PuluginId) == "" {
			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("volume %q must set plugin_id", volume.Name)})
		}
		if volume.Type != "csi" && volume.Type != "host" {
			issues = append(issues, Issue{Field: field, Message: fmt.Sprintf("volume %q has unsupported type %q (expected csi or host)", volume.Name, volume.Type)})
		}
	}

	return issues
}

func decodeYAMLStrict(data string, target interface{}) error {
	decoder := yaml.NewDecoder(strings.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%s", cleanYAMLError(err.Error()))
	}
	return nil
}

// cleanYAMLError strips verbose yaml.v3 internals from error messages while
// preserving any leading "line N:" location information.
func cleanYAMLError(msg string) string {
	msg = strings.TrimPrefix(msg, "yaml: ")
	msg = strings.TrimPrefix(msg, "unmarshal errors:\n  ")

	// Simplify type mismatch errors, e.g.
	//   "line 2: cannot unmarshal !!str `paul-te...` into map[string]interface {}"
	// becomes "line 2: unexpected value type"
	if before, _, found := strings.Cut(msg, ": cannot unmarshal"); found {
		return before + ": unexpected value type"
	}

	return msg
}

// --- yaml.Node helpers ---

// docMapping returns the root mapping node of a decoded YAML document, or nil.
func docMapping(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	return root.Content[0]
}

func scalarValue(n *yaml.Node) string {
	if n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}

// validateStringList validates each scalar item of a sequence node with the
// supplied function, attaching the item's line to any issue. Non-sequence nodes
// and non-scalar items produce structural issues with the relevant line.
func validateStringList(node *yaml.Node, field string, validate func(string) error) []Issue {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.ScalarNode && node.Value == "" {
		return nil // null / omitted list
	}
	if node.Kind != yaml.SequenceNode {
		return []Issue{{Field: "job", Line: node.Line, Message: fmt.Sprintf("%s must be a list", field)}}
	}
	var issues []Issue
	for _, item := range node.Content {
		if item.Kind != yaml.ScalarNode {
			issues = append(issues, Issue{Field: "job", Line: item.Line, Message: fmt.Sprintf("%s entries must be strings", field)})
			continue
		}
		if err := validate(item.Value); err != nil {
			issues = append(issues, Issue{Field: "job", Line: item.Line, Message: err.Error()})
		}
	}
	return issues
}

// lineFromError extracts a leading "line N" location from an error message, e.g.
// "yaml: line 5: ..." -> 5. Returns 0 when no line is present.
func lineFromError(err error) int {
	if err == nil {
		return 0
	}
	s := err.Error()
	const marker = "line "
	idx := strings.Index(s, marker)
	if idx < 0 {
		return 0
	}
	rest := s[idx+len(marker):]
	var n int
	for _, c := range rest {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// --- field validators ---

func validatePortMapping(port string) error {
	parts := strings.Split(port, ":")
	if len(parts) != 2 {
		return fmt.Errorf("port %q must be in the format hostPort:containerPort[/protocol]", port)
	}

	if err := validatePortNumber(parts[0], "host"); err != nil {
		return fmt.Errorf("port %q: %w", port, err)
	}

	containerPort := parts[1]
	if strings.Contains(containerPort, "/") {
		subParts := strings.Split(containerPort, "/")
		if len(subParts) != 2 {
			return fmt.Errorf("port %q must be in the format hostPort:containerPort[/protocol]", port)
		}
		containerPort = subParts[0]
		protocol := strings.ToLower(subParts[1])
		if protocol != "tcp" && protocol != "udp" {
			return fmt.Errorf("port %q must use the tcp or udp protocol", port)
		}
	}

	if err := validatePortNumber(containerPort, "container"); err != nil {
		return fmt.Errorf("port %q: %w", port, err)
	}

	return nil
}

func validatePortNumber(value, side string) error {
	number, err := strconv.Atoi(value)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("invalid %s port %q (expected 1-65535)", side, value)
	}
	return nil
}

func validateHostContainerPair(value, kind string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("%s %q must be in the format hostPath:containerPath", kind, value)
	}
	return nil
}

func validateEnvEntry(value string) error {
	if !strings.Contains(value, "=") {
		return fmt.Errorf("environment entry %q must be in the format KEY=value", value)
	}
	key := strings.SplitN(value, "=", 2)[0]
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("environment entry %q must set a variable name", value)
	}
	return nil
}

func validateAddHostEntry(value string) error {
	parts := strings.Split(value, ":")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" {
		return fmt.Errorf("add_host entry %q must be in the format hostname:ip", value)
	}
	if net.ParseIP(parts[1]) == nil {
		return fmt.Errorf("add_host entry %q must use a valid IP address", value)
	}
	return nil
}

func validateIPEntry(value, field string) error {
	if net.ParseIP(value) == nil {
		return fmt.Errorf("%s entry %q must be a valid IP address", field, value)
	}
	return nil
}

// validateCapability checks a Linux capability name, e.g. CAP_NET_ADMIN.
func validateCapability(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return fmt.Errorf("capability is empty")
	}
	if !strings.HasPrefix(v, "CAP_") {
		return fmt.Errorf("capability %q must start with CAP_ (e.g. CAP_NET_ADMIN)", v)
	}
	name := v[len("CAP_"):]
	if name == "" {
		return fmt.Errorf("capability %q is missing the name after CAP_", v)
	}
	for _, r := range name {
		if !((r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return fmt.Errorf("capability %q must be uppercase letters, digits and underscores after CAP_", v)
		}
	}
	return nil
}
