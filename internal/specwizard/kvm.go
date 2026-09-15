package specwizard

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// KVM virtual machine YAML
// ---------------------------------------------------------------------------

// kvmJobSpec mirrors the struct in internal/container/kvm/spaces.go.
// Duplicated for the same import-hygiene reason as the container jobSpec —
// the shape MUST stay in sync.
type kvmJobSpec struct {
	Name        string        `yaml:"name,omitempty"`
	Hostname    string        `yaml:"hostname,omitempty"`
	Image       string        `yaml:"image"`
	Memory      string        `yaml:"memory,omitempty"`
	CPUs        interface{}   `yaml:"cpus,omitempty"`
	Disk        string        `yaml:"disk,omitempty"`
	Network     *networkBlock `yaml:"network,omitempty"`
	Devices     []string      `yaml:"devices,omitempty"`
	Environment []string      `yaml:"environment,omitempty"`
}

// networkBlock is the job spec's `network:` mapping — mirrors the spec's
// apiclient.SpecKvmNetwork and the template's derived KVM fields.
type networkBlock struct {
	Mode         string `yaml:"mode,omitempty"`
	Cidr         string `yaml:"cidr,omitempty"`
	Bridge       string `yaml:"bridge,omitempty"`
	IPRangeStart string `yaml:"ip_range_start,omitempty"`
	IPRangeEnd   string `yaml:"ip_range_end,omitempty"`
	Gateway      string `yaml:"gateway,omitempty"`
}

func networkFromSpec(n *apiclient.SpecKvmNetwork) *networkBlock {
	if n == nil {
		return nil
	}
	if n.Mode == "nat" {
		// NAT never emits static addressing — even if a stale spec carries
		// values (e.g. the mode was switched after a bridged edit), they
		// must not reach the YAML.
		return &networkBlock{Mode: n.Mode, Bridge: n.Bridge}
	}
	return &networkBlock{
		Mode:         n.Mode,
		Cidr:         n.Cidr,
		Bridge:       n.Bridge,
		IPRangeStart: n.IPRangeStart,
		IPRangeEnd:   n.IPRangeEnd,
		Gateway:      n.Gateway,
	}
}

// ParseKvmYAML converts a KVM VM spec into UnifiedSpec. KVM templates cannot
// define volumes, so any volume text makes the spec non-wizardable.
func ParseKvmYAML(job, volumes string) (spec *apiclient.UnifiedSpec, wizardable bool, reason string) {
	if strings.TrimSpace(volumes) != "" {
		return nil, false, "KVM templates cannot define volumes"
	}

	if strings.TrimSpace(job) == "" {
		// An empty job is parseable — the wizard writes a fresh spec,
		// seeded with an empty network block for the VM Network inputs.
		return &apiclient.UnifiedSpec{KvmNetwork: &apiclient.SpecKvmNetwork{}}, true, ""
	}

	var root yaml.Node
	if err := yaml.Unmarshal([]byte(job), &root); err != nil {
		return nil, false, fmt.Sprintf("KVM YAML parse failed: %s", err.Error())
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return &apiclient.UnifiedSpec{KvmNetwork: &apiclient.SpecKvmNetwork{}}, true, ""
	}
	mapping := root.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return nil, false, "KVM YAML must be a mapping"
	}

	var js kvmJobSpec
	if err := mapping.Decode(&js); err != nil {
		return nil, false, fmt.Sprintf("KVM YAML parse failed: %s", err.Error())
	}

	spec = &apiclient.UnifiedSpec{
		Name:        js.Name,
		Image:       js.Image,
		Hostname:    js.Hostname,
		Memory:      js.Memory,
		CPUs:        interfaceToString(js.CPUs),
		Disk:        js.Disk,
		Environment: parseKeyValueStrings(js.Environment),
		HostDevices: js.Devices,
		KvmNetwork:  &apiclient.SpecKvmNetwork{},
	}
	if js.Network != nil {
		spec.KvmNetwork = &apiclient.SpecKvmNetwork{
			Mode:         js.Network.Mode,
			Cidr:         js.Network.Cidr,
			Bridge:       js.Network.Bridge,
			IPRangeStart: js.Network.IPRangeStart,
			IPRangeEnd:   js.Network.IPRangeEnd,
			Gateway:      js.Network.Gateway,
		}
	}
	return spec, true, ""
}

// BuildKvmYAML converts a UnifiedSpec back into KVM YAML. When the original
// job is valid YAML the known keys are patched in place (preserving comments
// and any fields outside the wizard's surface); otherwise the spec is
// regenerated from scratch.
func BuildKvmYAML(spec *apiclient.UnifiedSpec, originalJob, originalVolumes string) (job, volumes string, err error) {
	if spec == nil {
		return "", "", fmt.Errorf("nil spec")
	}

	js := kvmJobSpec{
		Name:        spec.Name,
		Hostname:    spec.Hostname,
		Image:       spec.Image,
		Memory:      spec.Memory,
		CPUs:        stringToNumericInterface(spec.CPUs),
		Disk:        spec.Disk,
		Network:     networkFromSpec(spec.KvmNetwork),
		Devices:     spec.HostDevices,
		Environment: keyValueStrings(spec.Environment),
	}

	if strings.TrimSpace(originalJob) != "" {
		var root yaml.Node
		if err := yaml.Unmarshal([]byte(originalJob), &root); err == nil &&
			root.Kind == yaml.DocumentNode && len(root.Content) > 0 &&
			root.Content[0].Kind == yaml.MappingNode {
			patchKvmMapping(root.Content[0], &js)
			out, marshalErr := yaml.Marshal(&root)
			if marshalErr != nil {
				return "", "", fmt.Errorf("marshal KVM YAML: %w", marshalErr)
			}
			return string(out), "", nil
		}
	}

	out, err := yaml.Marshal(&js)
	if err != nil {
		return "", "", fmt.Errorf("marshal KVM YAML: %w", err)
	}
	return string(out), "", nil
}

// patchKvmMapping rewrites the wizard-controlled keys of a decoded KVM job
// mapping in place. Keys whose new value is empty are removed; keys absent
// from the original are appended. Unknown keys are left untouched so the
// user's hand-edits survive a wizard round-trip.
func patchKvmMapping(mapping *yaml.Node, js *kvmJobSpec) {
	scalars := map[string]string{
		"name":     js.Name,
		"hostname": js.Hostname,
		"image":    js.Image,
		"memory":   js.Memory,
		"cpus":     strings.TrimSpace(interfaceToString(js.CPUs)),
		"disk":     js.Disk,
	}

	content := mapping.Content
	for i := 0; i+1 < len(content); i += 2 {
		key := content[i]
		if key.Kind != yaml.ScalarNode {
			continue
		}
		value, known := scalars[key.Value]
		if !known {
			continue
		}
		if value == "" {
			// Drop the key node and its value.
			mapping.Content = append(mapping.Content[:i], mapping.Content[i+2:]...)
			content = mapping.Content
			i -= 2
			continue
		}
		replaceScalarNode(content[i+1], kvmScalarValue(key.Value, value))
	}

	// Append keys the original didn't have.
	for _, key := range []string{"name", "hostname", "image", "memory", "cpus", "disk"} {
		if scalars[key] == "" {
			continue
		}
		if !kvmMappingHasKey(mapping, key) {
			value := kvmScalarValue(key, scalars[key])
			mapping.Content = append(mapping.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
				value,
			)
		}
	}

	// Devices is replaced wholesale (a plain string sequence).
	if idx := kvmMappingKeyIndex(mapping, "devices"); idx >= 0 {
		if len(js.Devices) == 0 {
			mapping.Content = append(mapping.Content[:idx], mapping.Content[idx+2:]...)
		} else {
			mapping.Content[idx+1] = kvmEnvSequence(js.Devices)
		}
	} else if len(js.Devices) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "devices"},
			kvmEnvSequence(js.Devices),
		)
	}

	// Environment is replaced wholesale.
	if idx := kvmMappingKeyIndex(mapping, "environment"); idx >= 0 {
		if len(js.Environment) == 0 {
			mapping.Content = append(mapping.Content[:idx], mapping.Content[idx+2:]...)
		} else {
			mapping.Content[idx+1] = kvmEnvSequence(js.Environment)
		}
	} else if len(js.Environment) > 0 {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "environment"},
			kvmEnvSequence(js.Environment),
		)
	}

	// The network block is replaced wholesale when the wizard carries one —
	// its sub-fields all come from the VM Network inputs together.
	if js.Network != nil {
		if netNode := kvmNetworkNode(js.Network); netNode != nil {
			if idx := kvmMappingKeyIndex(mapping, "network"); idx >= 0 {
				mapping.Content[idx+1] = netNode
			} else {
				mapping.Content = append(mapping.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "network"},
					netNode,
				)
			}
		}
	}
}

// kvmNetworkNode encodes the network block as a mapping node. Node.Encode
// yields the value node directly (no document wrapper), so the node itself
// is used — reaching for Content[0] would grab the mapping's first key.
func kvmNetworkNode(network *networkBlock) *yaml.Node {
	var node yaml.Node
	if err := node.Encode(network); err != nil {
		return nil
	}
	n := &node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// kvmScalarValue builds a scalar node for a wizard field. cpus is numeric by
// convention (the fresh-build path emits it unquoted via
// stringToNumericInterface) — carrying an int/float tag here keeps a patched
// spec byte-compatible with a regenerated one instead of growing quotes.
func kvmScalarValue(key, value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	if key == "cpus" {
		switch stringToNumericInterface(value).(type) {
		case int64:
			node.Tag = "!!int"
		case float64:
			node.Tag = "!!float"
		}
	}
	return node
}

// replaceScalarNode overwrites a scalar node's kind/tag/value in place.
func replaceScalarNode(node *yaml.Node, replacement *yaml.Node) {
	node.Kind = yaml.ScalarNode
	node.Tag = replacement.Tag
	node.Value = replacement.Value
	node.Style = 0
}

func kvmMappingHasKey(mapping *yaml.Node, key string) bool {
	return kvmMappingKeyIndex(mapping, key) >= 0
}

func kvmMappingKeyIndex(mapping *yaml.Node, key string) int {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Kind == yaml.ScalarNode && mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func kvmEnvSequence(entries []string) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, entry := range entries {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry})
	}
	return seq
}
