package specwizard

import (
	"strings"

	"github.com/paularlott/knot/apiclient"
	"gopkg.in/yaml.v3"
)

// CheckFullyRepresentable inspects a successfully-parsed spec and its raw
// source texts to determine whether the wizard can display everything. It
// returns fully=true when the wizard covers every field; otherwise it returns
// false with a human-readable reason listing what was detected outside the
// wizard's surface.
//
// This is separate from the wizardable check (which refuses to parse at all
// for multi-task jobs etc.) — a spec can be wizardable (parseable, editable)
// without being fully representable (the user would miss things if they only
// used wizard mode).
func CheckFullyRepresentable(platform, job, volumes string, spec *apiclient.UnifiedSpec) (fully bool, reason string) {
	switch {
	case isContainerPlatform(platform):
		return checkContainerRepresentable(job)
	case platform == "nomad":
		return checkNomadRepresentable(job)
	}
	return true, ""
}

func isContainerPlatform(p string) bool {
	return p == "docker" || p == "podman" || p == "apple" || p == "container"
}

// containerWizardFields is the set of top-level YAML keys the wizard controls
// for container specs. Any key outside this set means the spec has content the
// wizard can't show.
var containerWizardFields = map[string]bool{
	"container_name": true,
	"hostname":       true,
	"image":          true,
	"auth":           true,
	"ports":          true,
	"volumes":        true,
	"command":        true,
	"privileged":     true,
	"network":        true,
	"environment":    true,
	"cap_add":        true,
	"cap_drop":       true,
	"devices":        true,
	"dns":            true,
	"add_host":       true,
	"dns_search":     true,
	"memory":         true,
	"cpus":           true,
}

func checkContainerRepresentable(job string) (bool, string) {
	if strings.TrimSpace(job) == "" {
		return true, ""
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(job), &root); err != nil {
		return true, "" // parse failures are handled by wizardable check
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return true, ""
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return true, ""
	}

	var unknown []string
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		if !containerWizardFields[key.Value] {
			unknown = append(unknown, key.Value)
		}
	}
	if len(unknown) == 0 {
		return true, ""
	}
	return false, "spec contains fields outside the wizard: " + strings.Join(unknown, ", ")
}

// nomadWizardTaskBlocks is the set of block names inside a task that the
// wizard controls. Anything else (template, constraint, artifact, lifecycle,
// meta, etc.) means the spec has content outside the wizard.
var nomadWizardTaskBlocks = map[string]bool{
	"config":       true,
	"env":          true,
	"resources":    true,
	"volume_mount": true,
	"service":      true,
	"driver":       true, // field, not block, but appears at task level
}

// nomadWizardGroupBlocks is the set of block/field names at the group level
// the wizard controls.
var nomadWizardGroupBlocks = map[string]bool{
	"network": true,
	"volume":  true,
	"task":    true,
	"count":   true,
}

func checkNomadRepresentable(job string) (bool, string) {
	if strings.TrimSpace(job) == "" {
		return true, ""
	}

	var reasons []string

	// Check the task block for unrecognised children.
	taskRange, ok := findFirstTaskBlock(job)
	if !ok {
		return true, "" // no task block is handled by wizardable check
	}
	taskBody := job[taskRange.start:taskRange.end]
	if extras := findUnknownBlocks(taskBody, nomadWizardTaskBlocks); len(extras) > 0 {
		reasons = append(reasons, "task contains: "+strings.Join(extras, ", "))
	}

	// Check the group block for unrecognised children.
	groupRange, ok := findFirstGroupBlock(job)
	if ok {
		groupBody := job[groupRange.start:groupRange.end]
		if extras := findUnknownBlocks(groupBody, nomadWizardGroupBlocks); len(extras) > 0 {
			reasons = append(reasons, "group contains: "+strings.Join(extras, ", "))
		}
	}

	if len(reasons) == 0 {
		return true, ""
	}
	return false, "spec contains blocks outside the wizard (" + strings.Join(reasons, "; ") + ")"
}

// findUnknownBlocks scans a block body for field assignments and sub-blocks
// whose names aren't in the known set, at depth 0 only (doesn't recurse into
// nested blocks). It uses line-by-line heuristic parsing looking for patterns
// like `name {`, `name "label" {`, and `name = value` while tracking brace
// depth so the internals of nested blocks are skipped.
func findUnknownBlocks(body string, known map[string]bool) []string {
	seen := map[string]bool{}
	depth := 0
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Track brace depth: only examine lines at depth 0.
		opens := strings.Count(trimmed, "{")
		closes := strings.Count(trimmed, "}")
		if depth > 0 {
			depth += opens - closes
			if depth < 0 {
				depth = 0
			}
			continue
		}
		// At depth 0 — check for unknown names.
		if trimmed == "}" {
			continue
		}
		name := firstWord(trimmed)
		if name == "" || known[name] {
			depth += opens - closes
			continue
		}
		rest := strings.TrimSpace(trimmed[len(name):])
		if rest == "" {
			depth += opens - closes
			continue
		}
		if rest[0] == '{' || rest[0] == '"' || rest[0] == '=' {
			seen[name] = true
		}
		depth += opens - closes
		if depth < 0 {
			depth = 0
		}
	}
	var result []string
	for k := range seen {
		result = append(result, k)
	}
	return result
}

func firstWord(s string) string {
	for i, c := range s {
		if c == ' ' || c == '\t' || c == '{' || c == '=' || c == '"' {
			return s[:i]
		}
	}
	return s
}
