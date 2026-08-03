package apiclient

import "context"

// This file defines the unified JSON schema used by the template spec wizard.
// The wizard operates on UnifiedSpec regardless of the underlying platform
// (local container YAML or Nomad HCL); the parse/build endpoints in
// internal/api/specwizard.go translate between UnifiedSpec and the platform's
// native spec format.
//
// Design intent:
//   - The wizard only knows about UnifiedSpec, never about HCL or YAML directly.
//   - On Apply, the wizard posts {original native spec, edited UnifiedSpec} to
//     /api/spec/build, which performs source-position-aware patching of the
//     native spec so the user's hand-edits outside the wizard's fields are
//     preserved byte-for-byte.
//   - Fields not represented here (e.g.Nomad constraint blocks, multicast
//     config) survive because the patcher only touches the fields it knows.

// SpecParseRequest is the input to POST /api/spec/parse.
type SpecParseRequest struct {
	Platform string `json:"platform"`
	Job      string `json:"job"`
	Volumes  string `json:"volumes"`
}

// SpecParseResponse is the output of POST /api/spec/parse.
type SpecParseResponse struct {
	// Wizardable is false when the spec can't be safely presented in the
	// wizard (e.g. multi-task Nomad job, parse failure, unsupported driver).
	// When false, Reason carries a human-readable explanation and Spec is
	// nil.
	Wizardable bool         `json:"wizardable"`
	Reason     string       `json:"reason,omitempty"`
	Spec       *UnifiedSpec `json:"spec,omitempty"`

	// FullyRepresentable is true when every field in the spec is visible in
	// the wizard. When false, the template form should default to Advanced
	// mode (showing the raw textareas) because the spec contains constructs
	// outside the wizard's surface — e.g. Nomad template/constraint/artifact
	// blocks, docker mount {} stanzas, or unknown top-level container YAML
	// fields. The spec is still parseable and the wizard still works for the
	// fields it controls, but the user would miss things in wizard-only mode.
	// AdvancedReason explains what was detected.
	FullyRepresentable bool   `json:"fully_representable"`
	AdvancedReason     string `json:"advanced_reason,omitempty"`
}

// SpecBuildRequest is the input to POST /api/spec/build.
//
// On entry, OriginalJob / OriginalVolumes hold the current native spec text
// (which may have been hand-edited since the last parse) and Spec holds the
// wizard's edited unified representation. The build endpoint patches the
// native text in place for each field the wizard controls.
type SpecBuildRequest struct {
	Platform        string       `json:"platform"`
	OriginalJob     string       `json:"original_job"`
	OriginalVolumes string       `json:"original_volumes"`
	Spec            *UnifiedSpec `json:"spec"`
}

// SpecBuildResponse is the output of POST /api/spec/build.
type SpecBuildResponse struct {
	Job     string `json:"job"`
	Volumes string `json:"volumes"`
}

// CapabilityEntry is one Linux capability in the wizard's picker catalog.
// Name is the canonical CAP_UPPER_SNAKE form; Common marks the capabilities
// dev spaces most often need so the UI can float them to the top of the list.
type CapabilityEntry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Common      bool   `json:"common,omitempty"`
}

// CapabilitiesResponse is the output of GET /api/capabilities. The catalog is
// the common subset only — the wizard accepts any well-formed CAP_* name that
// already exists in the spec, and the raw editor can express the rest.
type CapabilitiesResponse struct {
	Capabilities []CapabilityEntry `json:"capabilities"`
}

// BaseImageRefreshResponse is the output of POST /api/base-images/refresh.
// Updated is true when the fetched manifest was newer than the active one and
// replaced it (and was therefore gossiped to the cluster). When false the
// server already had an equal or newer manifest and nothing changed.
type BaseImageRefreshResponse struct {
	Updated        bool   `json:"updated"`
	ActiveVersion  string `json:"active_version"`
	UpdateURL      string `json:"update_url,omitempty"`
	FetchedVersion string `json:"fetched_version,omitempty"`
}

// RefreshBaseImages forces the server to fetch the base image manifest from
// its configured update URL right now. The server gossips the result to the
// cluster when the fetched copy is newer than the active one.
func (c *ApiClient) RefreshBaseImages(ctx context.Context) (*BaseImageRefreshResponse, int, error) {
	response := &BaseImageRefreshResponse{}
	code, err := c.httpClient.Post(ctx, "/api/base-images/refresh", nil, response, 200)
	return response, code, err
}

// UnifiedSpec is the runtime-agnostic representation the wizard edits.
//
// Strings here frequently contain template variables like
// ${{ .server.base_image_registry }}. The wizard MUST NOT attempt to resolve
// these — they're resolved at deploy time via model.ResolveVariables.
type UnifiedSpec struct {
	// Name is the container's `container_name` (Docker/Podman/Apple) or the
	// Nomad job's label (job "<name>" { ... }). It is unrelated to Hostname —
	// templates commonly set both to different values, e.g.
	// container_name: ${{ .user.username }}-${{ .space.name }} alongside
	// hostname: ${{ .space.name }}.
	Name        string          `json:"name,omitempty"`
	Image       string          `json:"image"`
	Hostname    string          `json:"hostname,omitempty"`
	Command     []string        `json:"command,omitempty"`
	Environment []KeyValue      `json:"environment,omitempty"`
	Ports       []PortMapping   `json:"ports,omitempty"`
	Devices     []HostContainer `json:"devices,omitempty"`
	ExtraHosts  []HostIP        `json:"extra_hosts,omitempty"`
	DNS         []string        `json:"dns,omitempty"`
	DNSSearch   []string        `json:"dns_search,omitempty"`
	CapAdd      []string        `json:"cap_add,omitempty"`
	CapDrop     []string        `json:"cap_drop,omitempty"`
	Network     string          `json:"network,omitempty"`
	Privileged  bool            `json:"privileged,omitempty"`
	Memory      string `json:"memory,omitempty"`
	MemoryMax   string `json:"memory_max,omitempty"`
	CPUs        string `json:"cpus,omitempty"`

	// CPUType is Nomad-only and controls whether CPUs is emitted as `cpu`
	// (MHz, default) or `cores` (whole CPU cores). Empty defaults to "mhz".
	// Ignored by the container YAML emitter.
	CPUType string `json:"cpu_type,omitempty"`

	// Auth, when non-nil, toggles registry auth for the image pull. The
	// username/password are stored in the spec as-is; admins who want secret
	// handling should reference ${{ .var.* }} or ${{ .custom.* }} values.
	Auth *RegistryAuth `json:"auth,omitempty"`

	// Driver is the Nomad task driver ("docker" or "podman"). Empty for
	// container platforms. Controls how template mounts are emitted: docker
	// uses `mount {}` blocks, podman uses `volumes = [...]` entries.
	Driver string `json:"driver,omitempty"`

	// Storage is the unified list of mounts the wizard edits — see
	// StorageEntry. Each entry expands to everything needed for its Kind on
	// the target platform: a bind-mount line, a Volume Definition entry, and
	// (Nomad only) a `volume {}` stanza plus `volume_mount {}` — all kept in
	// sync from one row instead of three separately-edited artefacts.
	Storage []StorageEntry `json:"storage,omitempty"`

	// Templates holds Nomad `template {}` blocks (Nomad only). Each carries
	// the heredoc data, destination path, optional change_mode/change_signal,
	// and an optional Docker mount that binds the rendered file into the
	// container.
	Templates []NomadTemplate `json:"templates,omitempty"`
}

// NomadTemplate models a single Nomad `template {}` block as the wizard
// edits it. The destination path is the unique key. The data is the raw
// heredoc content (may contain knot template variables like ${{ .X }}).
// ChangeMode/ChangeSignal control what happens when the rendered file
// changes. MountTarget/MountReadonly, when set, cause the wizard to emit a
// matching `mount {}` block inside the docker-driver `config {}` so the
// rendered file is available inside the container.
type NomadTemplate struct {
	Destination   string `json:"destination"`
	Data          string `json:"data"`
	ChangeMode    string `json:"change_mode,omitempty"`
	ChangeSignal  string `json:"change_signal,omitempty"`
	MountTarget   string `json:"mount_target,omitempty"`
	MountReadonly bool   `json:"mount_readonly,omitempty"`
}

// KeyValue is a single environment variable or generic KEY=value pair.
type KeyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PortMapping describes a single host-to-container port binding. Protocol
// defaults to "tcp" when empty. Label carries the Nomad port-block name
// (e.g. "redis_port"); empty for local container YAML, which has no label
// concept.
type PortMapping struct {
	HostPort      int    `json:"host_port"`
	ContainerPort int    `json:"container_port"`
	Protocol      string `json:"protocol,omitempty"`
	Label         string `json:"label,omitempty"`
}

// HostContainer is a device mapping from host into the container.
type HostContainer struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	// CgroupPermissions is empty for the common case ("rwm").
	CgroupPermissions string `json:"cgroup_permissions,omitempty"`
}

// HostIP is an /etc/hosts style extra entry.
type HostIP struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
}

// RegistryAuth carries optional image pull credentials.
type RegistryAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// StorageEntry is one row of the wizard's unified Storage section. A single
// entry expands to everything needed to mount it on the target platform:
//
//   - Kind "bind": an existing host path, or the name of another StorageEntry
//     (kind "path" or "volume"), mounted directly. No definition is created —
//     HostPath is emitted as-is into the bind-mount list / volume_mount.
//   - Kind "path": a managed directory knot creates before the job starts and
//     removes with the space. HostPath is the absolute path; it is both the
//     definition (Volume Definition `paths:` entry) and the mount source.
//   - Kind "volume": a platform-managed named volume with a defined size/CSI
//     backing. Name identifies it; it appears in the Volume Definition
//     (`volumes:`) and, on Nomad, also gets a `volume {}` stanza in the group.
//
// ContainerPath and ReadOnly apply to every kind. The Nomad-only fields are
// ignored (and omitted on output) for container platforms.
type StorageEntry struct {
	Kind          string `json:"kind"` // "bind", "path", or "volume"
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only,omitempty"`

	// HostPath is the mount source for "bind" and "path" kinds: a literal
	// host path, or (bind only) the Name of another entry / a volume created
	// outside the wizard.
	HostPath string `json:"host_path,omitempty"`

	// Name identifies a "volume" kind entry and is what volume_mount / bind
	// entries reference to mount it elsewhere.
	Name string `json:"name,omitempty"`

	// Size is the requested size for a local container "volume" entry (Apple
	// Containers only; ignored by Docker/Podman, which don't size volumes).
	Size string `json:"size,omitempty"`

	// --- Nomad "volume" fields ---
	// VolumeType is "csi" or "host". PluginID, CapacityMin/Max and Namespace
	// mirror the CSI volume stanza. AccessModes lists the requested
	// access/attachment mode pairs (commonly one; CSI allows several).
	// MountFlags, Secrets and Parameters are passed through to Nomad as-is —
	// Parameters is required for host volumes (e.g. the mkdir plugin's mode/
	// uid/gid), Secrets for CSI plugins that need them.
	VolumeType  string             `json:"volume_type,omitempty"`
	PluginID    string             `json:"plugin_id,omitempty"`
	CapacityMin string             `json:"capacity_min,omitempty"`
	CapacityMax string             `json:"capacity_max,omitempty"`
	Namespace   string             `json:"namespace,omitempty"`
	FsType      string             `json:"fs_type,omitempty"`
	MountFlags  []string           `json:"mount_flags,omitempty"`
	AccessModes []VolumeCapability `json:"access_modes,omitempty"`
	Secrets     map[string]string  `json:"secrets,omitempty"`
	Parameters  map[string]string  `json:"parameters,omitempty"`
}

// VolumeCapability is one requested access/attachment mode pair for a Nomad
// CSI volume. Most volumes need exactly one; CSI allows a list so a volume
// can be requested with several acceptable combinations.
type VolumeCapability struct {
	AccessMode     string `json:"access_mode,omitempty"`
	AttachmentMode string `json:"attachment_mode,omitempty"`
}
