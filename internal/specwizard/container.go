// Package specwizard implements the conversion layer between knot's native
// template spec formats (local container YAML and Nomad HCL) and the unified
// representation the web wizard edits.
//
// The wizard never reads or writes native specs directly; it always goes via
// Parse→UnifiedSpec→Build. Build is source-position-aware: it patches only
// the fields the wizard knows about in the original native text, preserving
// anything outside that surface byte-for-byte.
package specwizard

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/database/model"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Local container YAML
// ---------------------------------------------------------------------------

// jobSpec mirrors the struct in internal/container/docker/spaces.go. Duplicated
// here so the specwizard package doesn't pull in the docker package's transitive
// imports (http client, etc.). The shape MUST stay in sync — see TestContainerYAMLRoundTrip.
type jobSpec struct {
	ContainerName string      `yaml:"container_name,omitempty"`
	Hostname      string      `yaml:"hostname,omitempty"`
	Image         string      `yaml:"image"`
	Auth          *authConfig `yaml:"auth,omitempty"`
	Ports         []string    `yaml:"ports,omitempty"`
	Volumes       []string    `yaml:"volumes,omitempty"`
	Command       []string    `yaml:"command,omitempty"`
	Privileged    bool        `yaml:"privileged,omitempty"`
	Network       string      `yaml:"network,omitempty"`
	Environment   []string    `yaml:"environment,omitempty"`
	CapAdd        []string    `yaml:"cap_add,omitempty"`
	CapDrop       []string    `yaml:"cap_drop,omitempty"`
	Devices       []string    `yaml:"devices,omitempty"`
	DNS           []string    `yaml:"dns,omitempty"`
	AddHost       []string    `yaml:"add_host,omitempty"`
	DNSSearch     []string    `yaml:"dns_search,omitempty"`
	Memory        string      `yaml:"memory,omitempty"`
	CPUs          interface{} `yaml:"cpus,omitempty"`
}

type authConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// ParseContainerYAML converts a local container YAML spec into UnifiedSpec.
// wizardable is false when the YAML can't be decoded, or when it uses
// constructs the comment-preserving patcher can't round-trip safely (multiple
// documents, anchors/aliases). The wizard UI uses that flag to keep the user in
// the raw editor rather than risk rewriting a spec it doesn't fully understand.
func ParseContainerYAML(job, volumes string) (spec *apiclient.UnifiedSpec, wizardable bool, reason string) {
	if strings.TrimSpace(job) == "" {
		// An empty job is parseable (the wizard will write a fresh spec).
		defs, defsReason := parseLocalStorageDefinitions(volumes)
		if defsReason != "" {
			return nil, false, defsReason
		}
		entries, storageReason := mergeContainerStorage(nil, defs)
		if storageReason != "" {
			return nil, false, storageReason
		}
		return &apiclient.UnifiedSpec{Storage: entries}, true, ""
	}

	doc, reason, err := inspectContainerYAML(job)
	if err != nil {
		return nil, false, fmt.Sprintf("container YAML parse failed: %s", err.Error())
	}
	if reason != "" {
		return nil, false, reason
	}
	if doc == nil {
		// Comments only — treat as a fresh spec.
		defs, defsReason := parseLocalStorageDefinitions(volumes)
		if defsReason != "" {
			return nil, false, defsReason
		}
		entries, storageReason := mergeContainerStorage(nil, defs)
		if storageReason != "" {
			return nil, false, storageReason
		}
		return &apiclient.UnifiedSpec{Storage: entries}, true, ""
	}

	var js jobSpec
	if err := doc.Decode(&js); err != nil {
		return nil, false, fmt.Sprintf("container YAML parse failed: %s", err.Error())
	}

	spec = &apiclient.UnifiedSpec{
		Name:        js.ContainerName,
		Image:       js.Image,
		Hostname:    js.Hostname,
		Command:     js.Command,
		Network:     js.Network,
		Privileged:  js.Privileged,
		Memory:      js.Memory,
		CPUs:        interfaceToString(js.CPUs),
		Ports:       parsePortStrings(js.Ports),
		Devices:     parseHostContainerStrings(js.Devices),
		Environment: parseKeyValueStrings(js.Environment),
		ExtraHosts:  parseHostIPStrings(js.AddHost),
		DNS:         js.DNS,
		DNSSearch:   js.DNSSearch,
		CapAdd:      NormaliseCapabilities(js.CapAdd),
		CapDrop:     NormaliseCapabilities(js.CapDrop),
	}
	if js.Auth != nil {
		spec.Auth = &apiclient.RegistryAuth{
			Username: js.Auth.Username,
			Password: js.Auth.Password,
		}
	}

	defs, defsReason := parseLocalStorageDefinitions(volumes)
	if defsReason != "" {
		return nil, false, defsReason
	}
	entries, storageReason := mergeContainerStorage(js.Volumes, defs)
	if storageReason != "" {
		return nil, false, storageReason
	}
	spec.Storage = entries
	return spec, true, ""
}

// mergeContainerStorage combines the bind-mount list (`volumes:` in the job
// YAML) with the volume/path definitions (Volume Definition YAML) into the
// wizard's unified Storage rows.
//
// A bind-mount entry whose source matches a defined volume name or managed
// path becomes a "volume"/"path" StorageEntry (carrying the mount's
// container_path/read_only plus the definition's fields); anything else stays
// a "bind" entry. Definitions with no matching mount are still surfaced —
// with an empty ContainerPath — so a defined-but-unmounted volume isn't
// silently dropped; the wizard shows it as unmounted and the user can fix it
// or leave it (unmounted volumes are unusual but not invalid).
//
// Returns reason non-empty when the definitions contain something the wizard
// can't safely round-trip (currently: nothing for containers, since
// LocalStorageSpec has no fields the wizard doesn't already model).
func mergeContainerStorage(bindStrings []string, defs *localStorageDefs) ([]apiclient.StorageEntry, string) {
	binds := make([]volumeBind, 0, len(bindStrings))
	for _, s := range bindStrings {
		vb, ok := parseVolumeBindString(s)
		if ok {
			binds = append(binds, vb)
		}
	}

	usedVolume := map[string]bool{}
	usedPath := map[string]bool{}
	var entries []apiclient.StorageEntry

	for _, vb := range binds {
		if defs != nil {
			if size, ok := defs.volumeSizes[vb.HostPath]; ok {
				usedVolume[vb.HostPath] = true
				entries = append(entries, apiclient.StorageEntry{
					Kind:          "volume",
					Name:          vb.HostPath,
					ContainerPath: vb.ContainerPath,
					ReadOnly:      vb.ReadOnly,
					Size:          size,
				})
				continue
			}
			if defs.hasPath(vb.HostPath) {
				usedPath[vb.HostPath] = true
				entries = append(entries, apiclient.StorageEntry{
					Kind:          "path",
					HostPath:      vb.HostPath,
					ContainerPath: vb.ContainerPath,
					ReadOnly:      vb.ReadOnly,
				})
				continue
			}
		}
		entries = append(entries, apiclient.StorageEntry{
			Kind:          "bind",
			HostPath:      vb.HostPath,
			ContainerPath: vb.ContainerPath,
			ReadOnly:      vb.ReadOnly,
		})
	}

	// Definitions with no matching mount: surface them unmounted rather than
	// drop them, so Apply doesn't silently delete a volume/path the user
	// defined but forgot to (or hasn't yet) mounted.
	if defs != nil {
		for _, name := range defs.volumeOrder {
			if usedVolume[name] {
				continue
			}
			entries = append(entries, apiclient.StorageEntry{
				Kind: "volume",
				Name: name,
				Size: defs.volumeSizes[name],
			})
		}
		for _, p := range defs.paths {
			if usedPath[p] {
				continue
			}
			entries = append(entries, apiclient.StorageEntry{Kind: "path", HostPath: p})
		}
	}

	if len(entries) == 0 {
		return nil, ""
	}
	return entries, ""
}

// inspectContainerYAML parses container YAML into its document nodes and
// reports whether the wizard can round-trip it.
//
// Returns:
//   - err when the text isn't valid YAML at all.
//   - reason (non-empty) when the YAML is valid but can't be safely patched:
//     multiple documents, a non-mapping root, or anchors/aliases. Re-encoding
//     any of those through yaml.Node would silently change or drop content.
//   - doc = the root mapping node, or nil when the document holds nothing but
//     comments.
func inspectContainerYAML(job string) (doc *yaml.Node, reason string, err error) {
	dec := yaml.NewDecoder(strings.NewReader(job))
	var docs []*yaml.Node
	for {
		var node yaml.Node
		decErr := dec.Decode(&node)
		if decErr == io.EOF {
			break
		}
		if decErr != nil {
			return nil, "", decErr
		}
		docs = append(docs, &node)
	}

	if len(docs) == 0 {
		return nil, "", nil
	}
	if len(docs) > 1 {
		return nil, "container YAML holds multiple documents (not editable via wizard)", nil
	}

	root := docs[0]
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, "", nil
		}
		if root.Content[0].Kind == yaml.ScalarNode && root.Content[0].Tag == "!!null" {
			return nil, "", nil
		}
	}

	body := root
	if body.Kind == yaml.DocumentNode {
		body = body.Content[0]
	}
	if body.Kind != yaml.MappingNode {
		return nil, "container YAML must be a mapping of fields (not editable via wizard)", nil
	}
	if hasAnchorsOrAliases(body) {
		return nil, "container YAML uses anchors or aliases (not editable via wizard)", nil
	}
	return root, "", nil
}

// hasAnchorsOrAliases reports whether any node in the tree declares an anchor
// (&name) or is an alias (*name), including merge keys (<<: *base). The
// patcher re-encodes the node tree, which expands aliases and drops the
// original structure, so the wizard refuses these documents outright.
func hasAnchorsOrAliases(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return true
	}
	for _, child := range node.Content {
		if hasAnchorsOrAliases(child) {
			return true
		}
	}
	return false
}

// BuildContainerYAML converts a UnifiedSpec back into container YAML plus the
// Volume Definition YAML. When originalJob is non-empty, the wizard's fields
// are patched into the original text via yaml.Node (preserving comments on
// scalar fields and field ordering for lines the wizard doesn't touch). When
// originalJob is empty, a fresh YAML is generated from the spec. The Volume
// Definition side (originalVolumes) is always patched independently via
// buildLocalStorageDefinitions, regardless of which path the job text takes.
//
// If the original text is valid YAML that can't be patched safely (multiple
// documents, anchors), Build errors instead of regenerating — regeneration
// would drop the parts of the file the wizard doesn't model. Text that isn't
// valid YAML at all carries nothing worth preserving, so it's replaced.
func BuildContainerYAML(spec *apiclient.UnifiedSpec, originalJob, originalVolumes string) (job, volumes string, err error) {
	if spec == nil {
		return "", "", fmt.Errorf("nil spec")
	}

	// Try to patch the original text (preserves comments on scalar fields).
	// Comments inside replaced list blocks (ports, volumes, environment) are
	// lost — the user accepted this tradeoff.
	if strings.TrimSpace(originalJob) != "" {
		_, reason, inspectErr := inspectContainerYAML(originalJob)
		switch {
		case inspectErr != nil:
			// Not valid YAML — fall through to full regeneration.
		case reason != "":
			return "", "", fmt.Errorf("%s", reason)
		default:
			patched, patchErr := patchContainerYAML(originalJob, spec)
			if patchErr != nil {
				return "", "", fmt.Errorf("patch container YAML: %w", patchErr)
			}
			job = patched
			volumes = buildLocalStorageDefinitions(spec.Storage, originalVolumes)
			return job, volumes, nil
		}
	}

	js := jobSpec{
		ContainerName: spec.Name,
		Image:         spec.Image,
		Hostname:      spec.Hostname,
		Command:       spec.Command,
		Network:       spec.Network,
		Privileged:    spec.Privileged,
		Memory:        spec.Memory,
		CPUs:          stringToNumericInterface(spec.CPUs),
		Ports:         portMappingStrings(spec.Ports),
		Volumes:       storageBindStrings(spec.Storage),
		Devices:       hostContainerStrings(spec.Devices),
		Environment:   keyValueStrings(spec.Environment),
		AddHost:       hostIPStrings(spec.ExtraHosts),
		DNS:           spec.DNS,
		DNSSearch:     spec.DNSSearch,
		CapAdd:        NormaliseCapabilities(spec.CapAdd),
		CapDrop:       NormaliseCapabilities(spec.CapDrop),
	}
	if spec.Auth != nil {
		js.Auth = &authConfig{
			Username: spec.Auth.Username,
			Password: spec.Auth.Password,
		}
	}

	out, err := yaml.Marshal(js)
	if err != nil {
		return "", "", fmt.Errorf("marshal container YAML: %w", err)
	}
	job = string(out)

	volumes = buildLocalStorageDefinitions(spec.Storage, originalVolumes)
	return job, volumes, nil
}

// stringToNumericInterface converts a numeric string like "6" or "0.5" to a
// float64 so yaml.v3 emits it unquoted (`cpus: 6` instead of `cpus: "6"`).
// Non-numeric strings are returned as-is so values like "auto" still work.
func stringToNumericInterface(s string) interface{} {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		// Return as int when the value is a whole number for cleaner output.
		if v == float64(int64(v)) {
			return int64(v)
		}
		return v
	}
	return s
}

// interfaceToString converts a decoded YAML scalar back to a string for the
// UnifiedSpec. Handles float64 (how yaml.v3 decodes unquoted numbers), int,
// and string.
func interfaceToString(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	return fmt.Sprintf("%v", v)
}

// ---------------------------------------------------------------------------
// String <-> struct helpers (shared shapes for the container schema)
// ---------------------------------------------------------------------------

func parsePortStrings(in []string) []apiclient.PortMapping {
	out := make([]apiclient.PortMapping, 0, len(in))
	for _, s := range in {
		pm, ok := parsePortMapping(s)
		if ok {
			out = append(out, pm)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// parsePortMapping accepts "host:container", "host:container/tcp",
// "container" and "container/tcp" forms.
func parsePortMapping(s string) (apiclient.PortMapping, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return apiclient.PortMapping{}, false
	}
	protocol := ""
	if i := strings.LastIndex(s, "/"); i != -1 {
		protocol = s[i+1:]
		s = s[:i]
	}
	parts := strings.Split(s, ":")
	var hostStr, contStr string
	switch len(parts) {
	case 1:
		hostStr, contStr = parts[0], parts[0]
	case 2:
		hostStr, contStr = parts[0], parts[1]
	default:
		return apiclient.PortMapping{}, false
	}
	host, err := strconv.Atoi(strings.TrimSpace(hostStr))
	if err != nil || host < 1 || host > 65535 {
		return apiclient.PortMapping{}, false
	}
	cont, err := strconv.Atoi(strings.TrimSpace(contStr))
	if err != nil || cont < 1 || cont > 65535 {
		return apiclient.PortMapping{}, false
	}
	return apiclient.PortMapping{HostPort: host, ContainerPort: cont, Protocol: protocol}, true
}

func portMappingStrings(in []apiclient.PortMapping) []string {
	var out []string
	for _, pm := range in {
		for _, proto := range expandProtocols(pm.Protocol) {
			s := fmt.Sprintf("%d:%d", pm.HostPort, pm.ContainerPort)
			s += "/" + proto
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// expandProtocols returns the list of protocols a port mapping should be
// emitted for. "tcp+udp" / "both" expands to both; anything else is returned
// as-is (defaulting to tcp when empty).
func expandProtocols(p string) []string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "", "tcp":
		return []string{"tcp"}
	case "udp":
		return []string{"udp"}
	case "tcp+udp", "both":
		return []string{"tcp", "udp"}
	}
	return []string{strings.ToLower(strings.TrimSpace(p))}
}

// volumeBind is a parsed entry from the container job YAML's `volumes:` list
// (a bind mount): "host:container[:mode]". It's an internal intermediate —
// mergeContainerStorage classifies each one into a StorageEntry Kind by
// cross-referencing the Volume Definition YAML.
type volumeBind struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

// parseVolumeBindString accepts "host:container[:mode]" form. mode is only
// checked for the "ro" flag — other modes (z, Z, nocopy, rw) round-trip via
// StorageEntry.ReadOnly=false and are otherwise not modelled by the wizard
// (matches specvalidate's validateVolumeBind, which accepts them without the
// wizard needing to distinguish them).
func parseVolumeBindString(s string) (volumeBind, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return volumeBind{}, false
	}
	parts := strings.Split(s, ":")
	if len(parts) < 2 {
		return volumeBind{}, false
	}
	vb := volumeBind{
		HostPath:      parts[0],
		ContainerPath: parts[1],
	}
	if len(parts) >= 3 {
		for _, flag := range strings.Split(parts[2], ",") {
			if strings.EqualFold(strings.TrimSpace(flag), "ro") {
				vb.ReadOnly = true
			}
		}
	}
	return vb, true
}

func (vb volumeBind) String() string {
	s := vb.HostPath + ":" + vb.ContainerPath
	if vb.ReadOnly {
		s += ":ro"
	}
	return s
}

// storageBindStrings renders the bind-mount side of every Storage entry as
// `volumes:` list strings for the container job YAML. All three Kinds mount
// the same way from the job's point of view — only "bind" lacks a Volume
// Definition entry.
func storageBindStrings(entries []apiclient.StorageEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.ContainerPath == "" {
			// Defined but not mounted (e.g. a volume kept for later use) —
			// nothing to emit into the job's bind-mount list.
			continue
		}
		source := storageMountSource(e)
		if source == "" {
			continue
		}
		vb := volumeBind{HostPath: source, ContainerPath: e.ContainerPath, ReadOnly: e.ReadOnly}
		out = append(out, vb.String())
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// storageMountSource returns the string a StorageEntry mounts as: its Name
// for "volume" entries, its HostPath for "bind"/"path" entries.
func storageMountSource(e apiclient.StorageEntry) string {
	if e.Kind == "volume" {
		return e.Name
	}
	return e.HostPath
}

func parseHostContainerStrings(in []string) []apiclient.HostContainer {
	out := make([]apiclient.HostContainer, 0, len(in))
	for _, s := range in {
		parts := strings.Split(s, ":")
		if len(parts) < 2 {
			continue
		}
		hc := apiclient.HostContainer{HostPath: parts[0], ContainerPath: parts[1]}
		if len(parts) >= 3 {
			hc.CgroupPermissions = parts[2]
		}
		out = append(out, hc)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hostContainerStrings(in []apiclient.HostContainer) []string {
	out := make([]string, 0, len(in))
	for _, hc := range in {
		s := hc.HostPath + ":" + hc.ContainerPath
		if hc.CgroupPermissions != "" {
			s += ":" + hc.CgroupPermissions
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseKeyValueStrings(in []string) []apiclient.KeyValue {
	envVars := container.ParseEnvStrings(in)
	out := make([]apiclient.KeyValue, 0, len(envVars))
	for _, ev := range envVars {
		out = append(out, apiclient.KeyValue{Key: ev.Key, Value: ev.Value})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func keyValueStrings(in []apiclient.KeyValue) []string {
	envVars := make([]container.EnvVar, 0, len(in))
	for _, kv := range in {
		envVars = append(envVars, container.EnvVar{Key: kv.Key, Value: kv.Value})
	}
	out := container.FormatEnvStrings(envVars)
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseHostIPStrings(in []string) []apiclient.HostIP {
	out := make([]apiclient.HostIP, 0, len(in))
	for _, s := range in {
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			continue
		}
		out = append(out, apiclient.HostIP{Hostname: parts[0], IP: parts[1]})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func hostIPStrings(in []apiclient.HostIP) []string {
	out := make([]string, 0, len(in))
	for _, hi := range in {
		out = append(out, hi.Hostname+":"+hi.IP)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ---------------------------------------------------------------------------
// Local container volume definitions (Volume Definition YAML field)
// ---------------------------------------------------------------------------

// localStorageDefs is the parsed Volume Definition YAML for a container
// platform, kept in insertion order so patching doesn't reshuffle the file.
type localStorageDefs struct {
	volumeOrder []string
	volumeSizes map[string]string
	paths       []string
}

func (d *localStorageDefs) hasPath(p string) bool {
	if d == nil {
		return false
	}
	for _, existing := range d.paths {
		if existing == p {
			return true
		}
	}
	return false
}

// parseLocalStorageDefinitions parses the Volume Definition YAML for a
// container platform (model.LocalStorageSpec's shape: `volumes:` map +
// `paths:` list). Returns reason non-empty when the YAML holds something the
// wizard can't safely round-trip — currently only structural parse failures,
// since LocalStorageSpec has no fields beyond what StorageEntry models.
func parseLocalStorageDefinitions(volumes string) (defs *localStorageDefs, reason string) {
	if strings.TrimSpace(volumes) == "" {
		return nil, ""
	}
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(volumes), &root); err != nil {
		return nil, fmt.Sprintf("volume definition parse failed: %s", err.Error())
	}
	mapping := docMappingNode(&root)
	if mapping == nil {
		return nil, ""
	}
	if hasAnchorsOrAliases(mapping) {
		return nil, "volume definition uses anchors or aliases (not editable via wizard)"
	}

	d := &localStorageDefs{volumeSizes: map[string]string{}}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		val := mapping.Content[i+1]
		switch key {
		case "volumes":
			if val.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				name := val.Content[j].Value
				size := ""
				entryVal := val.Content[j+1]
				if entryVal.Kind == yaml.MappingNode {
					for k := 0; k+1 < len(entryVal.Content); k += 2 {
						if entryVal.Content[k].Value == "size" {
							size = entryVal.Content[k+1].Value
						}
					}
				}
				d.volumeOrder = append(d.volumeOrder, name)
				d.volumeSizes[name] = size
			}
		case "paths":
			if val.Kind != yaml.SequenceNode {
				continue
			}
			for _, item := range val.Content {
				if item.Kind == yaml.ScalarNode && strings.TrimSpace(item.Value) != "" {
					d.paths = append(d.paths, item.Value)
				}
			}
		}
	}
	return d, ""
}

// docMappingNode returns the root mapping node of a decoded document, or nil
// for an empty/comment-only/non-mapping document.
func docMappingNode(root *yaml.Node) *yaml.Node {
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return nil
	}
	body := root.Content[0]
	if body.Kind == yaml.ScalarNode && body.Tag == "!!null" {
		return nil
	}
	if body.Kind != yaml.MappingNode {
		return nil
	}
	return body
}

// buildLocalStorageDefinitions renders the Volume Definition YAML for a
// container platform from the wizard's Storage entries. When originalVolumes
// is non-empty and still parseable, it's patched via yaml.Node to preserve
// comments and existing field order; otherwise it's regenerated fresh.
func buildLocalStorageDefinitions(entries []apiclient.StorageEntry, originalVolumes string) string {
	volumeNames, volumeSizes, paths := splitStorageDefinitions(entries)

	if strings.TrimSpace(originalVolumes) != "" {
		if patched, ok := patchLocalStorageDefinitions(originalVolumes, volumeNames, volumeSizes, paths); ok {
			return patched
		}
		// Falls through to full regeneration when the original text can't be
		// parsed as a YAML mapping (e.g. malformed input) — same tradeoff as
		// the container job YAML patcher.
	}

	if len(volumeNames) == 0 && len(paths) == 0 {
		return ""
	}
	lvs := localVolumeSpec{Paths: paths}
	if len(volumeNames) > 0 {
		lvs.Volumes = make(map[string]localVolumeEntry, len(volumeNames))
		for _, name := range volumeNames {
			lvs.Volumes[name] = localVolumeEntry{Size: volumeSizes[name]}
		}
	}
	out, err := yaml.Marshal(lvs)
	if err != nil {
		return ""
	}
	return string(out)
}

// splitStorageDefinitions extracts the "volume" and "path" kind entries from
// a Storage list into the shape the Volume Definition YAML needs. "bind"
// entries have no definition and are skipped.
func splitStorageDefinitions(entries []apiclient.StorageEntry) (volumeNames []string, volumeSizes map[string]string, paths []string) {
	volumeSizes = map[string]string{}
	for _, e := range entries {
		switch e.Kind {
		case "volume":
			if e.Name == "" {
				continue
			}
			volumeNames = append(volumeNames, e.Name)
			volumeSizes[e.Name] = e.Size
		case "path":
			if e.HostPath == "" {
				continue
			}
			paths = append(paths, e.HostPath)
		}
	}
	return
}

// localVolumeSpec mirrors model.LocalStorageSpec but lives here to keep the
// parse/build paths symmetric inside this package. The shape MUST match
// model.LocalStorageSpec.
type localVolumeSpec struct {
	Volumes map[string]localVolumeEntry `yaml:"volumes,omitempty"`
	Paths   []string                    `yaml:"paths,omitempty"`
}

type localVolumeEntry struct {
	Size string `yaml:"size,omitempty"`
}

// ---------------------------------------------------------------------------
// Platform dispatch
// ---------------------------------------------------------------------------

// Parse dispatches to the platform-specific parser. It returns wizardable=false
// for unknown platforms so the caller can communicate that to the UI.
//
// For Nomad, an HCLParser must be supplied (typically wrapping a Nomad client).
// A nil parser for the Nomad platform is treated as "Nomad not configured" and
// returns wizardable=false.
func Parse(platform, job, volumes string, hclParser HCLParser) (*apiclient.UnifiedSpec, bool, string) {
	switch platform {
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		return ParseContainerYAML(job, volumes)
	case model.PlatformNomad:
		return ParseNomadHCL(job, volumes, hclParser)
	default:
		return nil, false, "unsupported platform"
	}
}

// Build dispatches to the platform-specific builder. originalJob / originalVolumes
// are the current native spec text; both platforms now patch their Volume
// Definition YAML in place rather than regenerating it wholesale.
func Build(platform string, spec *apiclient.UnifiedSpec, originalJob, originalVolumes string) (job, volumes string, err error) {
	switch platform {
	case model.PlatformDocker, model.PlatformPodman, model.PlatformApple, model.PlatformContainer:
		return BuildContainerYAML(spec, originalJob, originalVolumes)
	case model.PlatformNomad:
		return BuildNomadHCL(spec, originalJob, originalVolumes)
	default:
		return "", "", fmt.Errorf("unsupported platform: %s", platform)
	}
}
