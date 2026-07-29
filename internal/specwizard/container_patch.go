package specwizard

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"gopkg.in/yaml.v3"
)

// patchContainerYAML patches the wizard-controlled fields into the original
// container YAML text using yaml.v3's Node API, preserving comments and
// field ordering for lines the wizard doesn't touch.
//
// Comments on scalar fields (image, memory, cpus, hostname, etc.) survive.
// Comments inside replaced list blocks (ports, volumes, environment, etc.)
// are lost — the user accepted this tradeoff.
//
// If the original text fails to parse as YAML, the caller should fall back
// to full regeneration via BuildContainerYAML.
func patchContainerYAML(original string, spec *apiclient.UnifiedSpec) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(original), &root); err != nil {
		return "", fmt.Errorf("parse original YAML: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return "", fmt.Errorf("not a YAML document")
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return "", fmt.Errorf("not a YAML mapping")
	}

	// Index existing keys → position in Content slice.
	existing := map[string]int{}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode {
			existing[mapping.Content[i].Value] = i
		}
	}

	// Fields to remove at the end (collected here so that inline removal
	// doesn't shift indices for subsequent patches).
	removed := map[string]bool{}

	// Helpers ---------------------------------------------------------------

	// setOrRemoveScalar updates a string field or marks it for removal when
	// value is empty. Removals are deferred to avoid index shifts.
	setOrRemoveScalar := func(key, value string) {
		if idx, ok := existing[key]; ok {
			if value == "" {
				removed[key] = true
			} else {
				mapping.Content[idx+1].Value = value
				mapping.Content[idx+1].Tag = "!!str"
			}
		} else if value != "" {
			appendMappingEntry(mapping, key, scalarStr(value))
		}
	}

	setOrRemoveNumeric := func(key, value string) {
		if idx, ok := existing[key]; ok {
			if value == "" {
				removed[key] = true
			} else {
				mapping.Content[idx+1] = scalarNumeric(value)
			}
		} else if value != "" {
			appendMappingEntry(mapping, key, scalarNumeric(value))
		}
	}

	// setBool updates an existing boolean field, but only adds a missing one
	// when the value is true — writing `privileged: false` into every spec the
	// wizard touches would be pure noise.
	setBool := func(key string, value bool) {
		if idx, ok := existing[key]; ok {
			mapping.Content[idx+1] = scalarBool(value)
		} else if value {
			appendMappingEntry(mapping, key, scalarBool(value))
		}
	}

	setOrRemoveSequence := func(key string, items []string) {
		if idx, ok := existing[key]; ok {
			if len(items) == 0 {
				removed[key] = true
			} else {
				mapping.Content[idx+1] = sequenceNode(items)
			}
		} else if len(items) > 0 {
			appendMappingEntry(mapping, key, sequenceNode(items))
		}
	}

	// Apply patches ---------------------------------------------------------

	setOrRemoveScalar("container_name", spec.Name)
	setOrRemoveScalar("image", spec.Image)
	setOrRemoveScalar("hostname", spec.Hostname)
	setOrRemoveScalar("memory", spec.Memory)
	setOrRemoveNumeric("cpus", spec.CPUs)
	setBool("privileged", spec.Privileged)
	setOrRemoveScalar("network", spec.Network)
	setOrRemoveSequence("ports", portMappingStrings(spec.Ports))
	setOrRemoveSequence("volumes", storageBindStrings(spec.Storage))
	setOrRemoveSequence("environment", keyValueStrings(spec.Environment))
	setOrRemoveSequence("devices", hostContainerStrings(spec.Devices))
	setOrRemoveSequence("add_host", hostIPStrings(spec.ExtraHosts))
	setOrRemoveSequence("dns", spec.DNS)
	setOrRemoveSequence("dns_search", spec.DNSSearch)
	setOrRemoveSequence("cap_add", NormaliseCapabilities(spec.CapAdd))
	setOrRemoveSequence("cap_drop", NormaliseCapabilities(spec.CapDrop))
	setOrRemoveSequence("command", spec.Command)

	// Auth block
	if spec.Auth != nil {
		authNode := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
			Content: []*yaml.Node{
				scalarStr("username"), scalarStr(spec.Auth.Username),
				scalarStr("password"), scalarStr(spec.Auth.Password),
			},
		}
		if idx, ok := existing["auth"]; ok {
			mapping.Content[idx+1] = authNode
		} else {
			appendMappingEntry(mapping, "auth", authNode)
		}
	}

	// Remove marked entries (deferred to avoid index shifts during patching).
	if len(removed) > 0 {
		var kept []*yaml.Node
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			keyNode := mapping.Content[i]
			if keyNode.Kind == yaml.ScalarNode && removed[keyNode.Value] {
				continue
			}
			kept = append(kept, mapping.Content[i], mapping.Content[i+1])
		}
		mapping.Content = kept
	}

	// Encode back
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return "", fmt.Errorf("encode patched YAML: %w", err)
	}
	enc.Close()
	return buf.String(), nil
}

// patchLocalStorageDefinitions patches the wizard's volume/path entries into
// the original Volume Definition YAML via yaml.Node, preserving comments and
// the existing order of untouched volumes. Returns ok=false when the original
// text can't be parsed as a YAML mapping (caller falls back to regenerating).
//
// Semantics mirror patchContainerYAML: existing volumes not mentioned by
// volumeNames are removed (the wizard is the source of truth for what's
// currently defined), existing ones keep their comments/position and just get
// their size updated, and new ones are appended in the order the caller
// supplied. paths is replaced wholesale, same as any other sequence field —
// comments inside that list are not preserved.
func patchLocalStorageDefinitions(original string, volumeNames []string, volumeSizes map[string]string, paths []string) (string, bool) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(original), &root); err != nil {
		return "", false
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return "", false
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return "", false
	}

	wanted := map[string]bool{}
	for _, n := range volumeNames {
		wanted[n] = true
	}

	// Patch (or build fresh) the `volumes:` mapping node.
	volumesIdx := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "volumes" {
			volumesIdx = i
			break
		}
	}

	if len(volumeNames) == 0 {
		if volumesIdx >= 0 {
			mapping.Content = removeMappingPairAt(mapping.Content, volumesIdx)
		}
	} else {
		var volumesNode *yaml.Node
		if volumesIdx >= 0 && mapping.Content[volumesIdx+1].Kind == yaml.MappingNode {
			volumesNode = mapping.Content[volumesIdx+1]
		} else {
			volumesNode = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			if volumesIdx >= 0 {
				mapping.Content[volumesIdx+1] = volumesNode
			} else {
				appendMappingEntry(mapping, "volumes", volumesNode)
			}
		}
		patchVolumesMapping(volumesNode, volumeNames, volumeSizes, wanted)
	}

	// Patch (or build fresh / remove) the `paths:` sequence.
	pathsIdx := -1
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "paths" {
			pathsIdx = i
			break
		}
	}
	if len(paths) == 0 {
		if pathsIdx >= 0 {
			mapping.Content = removeMappingPairAt(mapping.Content, pathsIdx)
		}
	} else if pathsIdx >= 0 {
		mapping.Content[pathsIdx+1] = sequenceNode(paths)
	} else {
		appendMappingEntry(mapping, "paths", sequenceNode(paths))
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return "", false
	}
	enc.Close()
	return buf.String(), true
}

// patchVolumesMapping updates volumesNode in place: existing entries in
// wanted keep their position/comments with size refreshed; entries not in
// wanted are dropped; names in volumeNames not already present are appended
// in order.
func patchVolumesMapping(volumesNode *yaml.Node, volumeNames []string, volumeSizes map[string]string, wanted map[string]bool) {
	existing := map[string]int{}
	for i := 0; i+1 < len(volumesNode.Content); i += 2 {
		existing[volumesNode.Content[i].Value] = i
	}

	// Update sizes on entries that survive.
	for name, idx := range existing {
		if !wanted[name] {
			continue
		}
		setVolumeEntrySize(volumesNode.Content[idx+1], volumeSizes[name])
	}

	// Drop entries no longer wanted.
	var kept []*yaml.Node
	for i := 0; i+1 < len(volumesNode.Content); i += 2 {
		if wanted[volumesNode.Content[i].Value] {
			kept = append(kept, volumesNode.Content[i], volumesNode.Content[i+1])
		}
	}
	volumesNode.Content = kept

	// Append new entries in caller order.
	for _, name := range volumeNames {
		if _, ok := existing[name]; ok {
			continue
		}
		appendMappingEntry(volumesNode, name, newVolumeEntryNode(volumeSizes[name]))
	}
}

// setVolumeEntrySize updates (or adds/removes) the `size` field of a single
// volume entry node in place.
func setVolumeEntrySize(entryNode *yaml.Node, size string) {
	if entryNode.Kind != yaml.MappingNode {
		if size == "" {
			return
		}
		entryNode.Kind = yaml.MappingNode
		entryNode.Tag = "!!map"
		entryNode.Value = ""
		entryNode.Content = nil
	}
	for i := 0; i+1 < len(entryNode.Content); i += 2 {
		if entryNode.Content[i].Value == "size" {
			if size == "" {
				entryNode.Content = removeMappingPairAt(entryNode.Content, i)
			} else {
				entryNode.Content[i+1].Value = size
				entryNode.Content[i+1].Tag = "!!str"
			}
			return
		}
	}
	if size != "" {
		appendMappingEntry(entryNode, "size", scalarStr(size))
	}
}

func newVolumeEntryNode(size string) *yaml.Node {
	if size == "" {
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	}
	return &yaml.Node{
		Kind:    yaml.MappingNode,
		Tag:     "!!map",
		Content: []*yaml.Node{scalarStr("size"), scalarStr(size)},
	}
}

// removeMappingPairAt removes the key/value pair starting at index idx (which
// must be even and point at a key node) from a mapping's Content slice.
func removeMappingPairAt(content []*yaml.Node, idx int) []*yaml.Node {
	if idx < 0 || idx+1 >= len(content) {
		return content
	}
	out := make([]*yaml.Node, 0, len(content)-2)
	out = append(out, content[:idx]...)
	out = append(out, content[idx+2:]...)
	return out
}

// --- yaml.Node helpers ---

func scalarStr(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v}
}

func scalarBool(v bool) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(v)}
}

// scalarNumeric emits an unquoted number when the string is numeric, falling
// back to a quoted string for non-numeric values.
func scalarNumeric(s string) *yaml.Node {
	s = strings.TrimSpace(s)
	if v, err := strconv.ParseInt(s, 10, 64); err == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(v, 10)}
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(v, 'f', -1, 64)}
	}
	return scalarStr(s)
}

func sequenceNode(items []string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, s := range items {
		node.Content = append(node.Content, scalarStr(s))
	}
	return node
}

// appendMappingEntry adds a key-value pair to the end of a mapping node.
func appendMappingEntry(mapping *yaml.Node, key string, valNode *yaml.Node) {
	mapping.Content = append(mapping.Content, scalarStr(key), valNode)
}

// removeMappingEntry removes the key-value pair at the given index. Caller
// must ensure idx is valid (even, points to a key node).
func removeMappingEntry(mapping *yaml.Node, idx int) {
	if idx+1 < len(mapping.Content) {
		mapping.Content = append(mapping.Content[:idx], mapping.Content[idx+2:]...)
	}
}

// Usage of apiclient import (avoids unused import when patchContainerYAML
// is the only consumer in this file).
var _ = (*apiclient.UnifiedSpec)(nil)
