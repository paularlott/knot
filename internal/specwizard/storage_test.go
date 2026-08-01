package specwizard

import (
	"strings"
	"testing"

	"github.com/paularlott/knot/apiclient"
)

// ---------------------------------------------------------------------------
// Container: StorageEntry parse/build round-trips
// ---------------------------------------------------------------------------

func TestContainerStorage_bindMountRoundTrip(t *testing.T) {
	original := `image: x:1
volumes:
  - /host/path:/container/path
  - data:/data:ro
`
	spec, wizardable, reason := ParseContainerYAML(original, "")
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	var binds []apiclient.StorageEntry
	for _, s := range spec.Storage {
		if s.Kind == "bind" {
			binds = append(binds, s)
		}
	}
	if len(binds) != 2 {
		t.Fatalf("expected 2 bind entries, got %d: %+v", len(binds), spec.Storage)
	}
	if binds[0].HostPath != "/host/path" || binds[0].ContainerPath != "/container/path" {
		t.Errorf("bind[0] = %+v", binds[0])
	}
	if binds[1].HostPath != "data" || binds[1].ContainerPath != "/data" || !binds[1].ReadOnly {
		t.Errorf("bind[1] = %+v", binds[1])
	}

	// Build and re-parse
	job, _, err := BuildContainerYAML(spec, original, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job, "/host/path:/container/path") {
		t.Errorf("bind mount lost:\n%s", job)
	}
	if !strings.Contains(job, "data:/data:ro") {
		t.Errorf("read-only bind mount lost:\n%s", job)
	}
}

func TestContainerStorage_volumeWithDefinitionRoundTrip(t *testing.T) {
	original := `image: x:1
volumes:
  - workspace:/workspace
`
	volumeDef := `volumes:
  workspace:
    size: 20G
`
	spec, wizardable, reason := ParseContainerYAML(original, volumeDef)
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	// workspace should be classified as "volume" because it matches the definition
	var vols []apiclient.StorageEntry
	for _, s := range spec.Storage {
		if s.Kind == "volume" {
			vols = append(vols, s)
		}
	}
	if len(vols) != 1 {
		t.Fatalf("expected 1 volume entry, got %d: %+v", len(vols), spec.Storage)
	}
	if vols[0].Name != "workspace" || vols[0].Size != "20G" || vols[0].ContainerPath != "/workspace" {
		t.Errorf("volume = %+v", vols[0])
	}

	// Build and verify both the job YAML and volume definition are generated
	job, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(job, "workspace:/workspace") {
		t.Errorf("volume mount missing from job:\n%s", job)
	}
	if !strings.Contains(volumes, "workspace") || !strings.Contains(volumes, "20G") {
		t.Errorf("volume definition missing:\n%s", volumes)
	}
}

func TestContainerStorage_pathRoundTrip(t *testing.T) {
	volumeDef := `paths:
  - /storage/${{ .space.id }}/data
`
	spec, wizardable, reason := ParseContainerYAML("image: x:1", volumeDef)
	if !wizardable {
		t.Fatalf("not wizardable: %s", reason)
	}

	var paths []apiclient.StorageEntry
	for _, s := range spec.Storage {
		if s.Kind == "path" {
			paths = append(paths, s)
		}
	}
	if len(paths) != 1 || paths[0].HostPath != "/storage/${{ .space.id }}/data" {
		t.Errorf("paths = %+v", paths)
	}

	// Build
	_, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(volumes, "/storage/${{ .space.id }}/data") {
		t.Errorf("path lost in volume definition:\n%s", volumes)
	}
}

func TestContainerStorage_mixedKindsRoundTrip(t *testing.T) {
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "bind", HostPath: "/etc/hosts", ContainerPath: "/etc/hosts", ReadOnly: true},
			{Kind: "volume", Name: "workspace", ContainerPath: "/workspace", Size: "10G"},
			{Kind: "path", HostPath: "/storage/data"},
		},
	}

	job, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Job should have bind mount strings for all mounted entries
	if !strings.Contains(job, "/etc/hosts:/etc/hosts:ro") {
		t.Errorf("bind mount missing from job:\n%s", job)
	}
	if !strings.Contains(job, "workspace:/workspace") {
		t.Errorf("volume mount missing from job:\n%s", job)
	}

	// Volume definition should have the volume and path
	if !strings.Contains(volumes, "workspace") {
		t.Errorf("volume name missing from definition:\n%s", volumes)
	}
	if !strings.Contains(volumes, "10G") {
		t.Errorf("volume size missing from definition:\n%s", volumes)
	}
	if !strings.Contains(volumes, "/storage/data") {
		t.Errorf("path missing from definition:\n%s", volumes)
	}
}

func TestContainerStorage_emptyStorageProducesNoVolumes(t *testing.T) {
	spec := &apiclient.UnifiedSpec{Image: "x:1"}
	job, volumes, err := BuildContainerYAML(spec, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(job, "volumes:") {
		t.Errorf("empty storage should not produce volumes in job:\n%s", job)
	}
	if volumes != "" {
		t.Errorf("empty storage should produce empty volume definition, got: %q", volumes)
	}
}

// ---------------------------------------------------------------------------
// Container: Volume definition comment preservation
// ---------------------------------------------------------------------------

func TestContainerStorage_volumeDefCommentPreserved(t *testing.T) {
	originalVolumes := `# My volumes
volumes:
  workspace:
    size: 20G  # workspace storage
paths:
  - /storage/data  # persistent data
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "workspace", ContainerPath: "/workspace", Size: "30G"},
			{Kind: "path", HostPath: "/storage/data"},
		},
	}
	_, volumes, err := BuildContainerYAML(spec, "image: x:1", originalVolumes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(volumes, "# My volumes") {
		t.Errorf("head comment lost:\n%s", volumes)
	}
	if !strings.Contains(volumes, "30G") {
		t.Errorf("size not updated:\n%s", volumes)
	}
}

func TestContainerStorage_volumeDefNewVolumeAdded(t *testing.T) {
	originalVolumes := `volumes:
  workspace:
    size: 20G
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "workspace", ContainerPath: "/workspace", Size: "20G"},
			{Kind: "volume", Name: "cache", ContainerPath: "/cache", Size: "5G"},
		},
	}
	_, volumes, err := BuildContainerYAML(spec, "image: x:1", originalVolumes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(volumes, "workspace") || !strings.Contains(volumes, "cache") {
		t.Errorf("volume not added:\n%s", volumes)
	}
	if !strings.Contains(volumes, "5G") {
		t.Errorf("new volume size missing:\n%s", volumes)
	}
}

func TestContainerStorage_volumeDefVolumeRemoved(t *testing.T) {
	originalVolumes := `volumes:
  workspace:
    size: 20G
  cache:
    size: 5G
`
	spec := &apiclient.UnifiedSpec{
		Image: "x:1",
		Storage: []apiclient.StorageEntry{
			{Kind: "volume", Name: "workspace", ContainerPath: "/workspace", Size: "20G"},
			// cache removed
		},
	}
	_, volumes, err := BuildContainerYAML(spec, "image: x:1", originalVolumes)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(volumes, "workspace") {
		t.Errorf("workspace lost:\n%s", volumes)
	}
	if strings.Contains(volumes, "cache") {
		t.Errorf("cache should be removed:\n%s", volumes)
	}
}

// ---------------------------------------------------------------------------
// Nomad: StorageEntry parse/build round-trips
// ---------------------------------------------------------------------------

func TestNomadStorage_volumeDefinitionRoundTrip(t *testing.T) {
	volumeDef := `volumes:
  - name: data
    type: csi
    plugin_id: hostpath
    capacity_min: 1G
    capacity_max: 10G
    mount_options:
      fs_type: ext4
      mount_flags:
        - rw
        - noatime
    capabilities:
      - access_mode: single-node-writer
        attachment_mode: file-system
    secrets:
      key: secret-value
    parameters:
      mode: "0755"
paths:
  - /storage/${{ .space.id }}/data
`
	entries := parseNomadVolumeDefinitionsToStorage(volumeDef)
	if entries == nil {
		t.Fatal("nil entries")
	}

	var vols, paths []apiclient.StorageEntry
	for _, e := range entries {
		switch e.Kind {
		case "volume":
			vols = append(vols, e)
		case "path":
			paths = append(paths, e)
		}
	}
	if len(vols) != 1 {
		t.Fatalf("vols = %+v", vols)
	}
	v := vols[0]
	if v.Name != "data" || v.PluginID != "hostpath" || v.VolumeType != "csi" {
		t.Errorf("basic fields: %+v", v)
	}
	if v.CapacityMin != "1G" || v.CapacityMax != "10G" {
		t.Errorf("capacity: %+v", v)
	}
	if v.FsType != "ext4" {
		t.Errorf("fs_type: %q", v.FsType)
	}
	if len(v.MountFlags) != 2 || v.MountFlags[0] != "rw" || v.MountFlags[1] != "noatime" {
		t.Errorf("mount_flags: %v", v.MountFlags)
	}
	if len(v.AccessModes) != 1 || v.AccessModes[0].AccessMode != "single-node-writer" {
		t.Errorf("access_modes: %+v", v.AccessModes)
	}
	if v.Secrets == nil || v.Secrets["key"] != "secret-value" {
		t.Errorf("secrets: %v", v.Secrets)
	}
	if v.Parameters == nil || v.Parameters["mode"] != "0755" {
		t.Errorf("parameters: %v", v.Parameters)
	}
	if len(paths) != 1 || paths[0].HostPath != "/storage/${{ .space.id }}/data" {
		t.Errorf("paths: %+v", paths)
	}

	// Build back and verify round-trip
	output := buildNomadStorageDefinitions(entries, "")
	if !strings.Contains(output, "hostpath") || !strings.Contains(output, "secret-value") {
		t.Errorf("fields lost in build:\n%s", output)
	}
	if !strings.Contains(output, "/storage/${{ .space.id }}/data") {
		t.Errorf("path lost in build:\n%s", output)
	}
}

func TestNomadStorage_mergeVolumeMountsWithDefinitions(t *testing.T) {
	// Simulate: job has volume_mount referencing "data", and Volume Definition
	// YAML defines "data" as a CSI volume.
	mounts := []apiclient.StorageEntry{
		{Kind: "bind", HostPath: "data", ContainerPath: "/data", ReadOnly: false},
		{Kind: "bind", HostPath: "unknown-ref", ContainerPath: "/unknown"},
	}
	volumeDef := `volumes:
  - name: data
    type: csi
    plugin_id: hostpath
    capacity_min: 1G
    capacity_max: 10G
`
	result := mergeNomadStorage(mounts, volumeDef)

	if len(result) < 2 {
		t.Fatalf("result = %+v", result)
	}

	// "data" should be reclassified as volume
	var dataEntry *apiclient.StorageEntry
	var unknownEntry *apiclient.StorageEntry
	for i := range result {
		switch result[i].HostPath {
		case "data":
			if result[i].Kind == "volume" {
				dataEntry = &result[i]
			}
		case "unknown-ref":
			unknownEntry = &result[i]
		}
		if result[i].Name == "data" && result[i].Kind == "volume" {
			dataEntry = &result[i]
		}
	}

	if dataEntry == nil {
		t.Fatalf("data entry not found or not reclassified as volume: %+v", result)
	}
	if dataEntry.PluginID != "hostpath" || dataEntry.ContainerPath != "/data" {
		t.Errorf("data entry wrong: %+v", dataEntry)
	}
	if unknownEntry == nil || unknownEntry.Kind != "bind" {
		t.Errorf("unknown-ref should stay as bind: %+v", result)
	}
}

func TestNomadStorage_emptyPreservesOriginalVolumes(t *testing.T) {
	original := "volumes:\n  - name: keep\n    type: csi\n    plugin_id: hostpath\n"
	output := buildNomadStorageDefinitions(nil, original)
	if output != original {
		t.Errorf("nil entries should preserve original:\nwant: %q\ngot:  %q", original, output)
	}
}

// ---------------------------------------------------------------------------
// Wizardability detection (representable.go)
// ---------------------------------------------------------------------------

func TestCheckFullyRepresentable_containerSimple(t *testing.T) {
	job := "image: x:1\nhostname: test\nmemory: 2G\n"
	fully, reason := CheckFullyRepresentable("docker", job, "", nil)
	if !fully {
		t.Errorf("simple container spec should be fully representable: %s", reason)
	}
}

func TestCheckFullyRepresentable_containerWithUnknownField(t *testing.T) {
	job := "image: x:1\nshell: bash\n"
	fully, reason := CheckFullyRepresentable("docker", job, "", nil)
	if fully {
		t.Error("spec with unknown field should not be fully representable")
	}
	if !strings.Contains(reason, "shell") {
		t.Errorf("reason should mention 'shell': %s", reason)
	}
}

func TestCheckFullyRepresentable_containerEmpty(t *testing.T) {
	fully, _ := CheckFullyRepresentable("docker", "", "", nil)
	if !fully {
		t.Error("empty spec should be fully representable")
	}
}

func TestCheckFullyRepresentable_nomadSimple(t *testing.T) {
	job := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      env { TZ = "UTC" }
      resources { memory = 2048 }
    }
  }
}
`
	fully, reason := CheckFullyRepresentable("nomad", job, "", nil)
	if !fully {
		t.Errorf("simple Nomad spec should be fully representable: %s", reason)
	}
}

func TestCheckFullyRepresentable_nomadWithTemplate(t *testing.T) {
	job := `job "demo" {
  group "app" {
    task "app" {
      driver = "docker"
      config { image = "x:1" }
      template {
        data = "secret"
        destination = "local/env"
      }
    }
  }
}
`
	fully, reason := CheckFullyRepresentable("nomad", job, "", nil)
	if !fully {
		t.Errorf("spec with template block should be fully representable: %s", reason)
	}
}

func TestCheckFullyRepresentable_nomadWithConstraint(t *testing.T) {
	job := `job "demo" {
  group "app" {
    constraint {
      attribute = "${attr.kernel.name}"
      value     = "linux"
    }
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	fully, reason := CheckFullyRepresentable("nomad", job, "", nil)
	if fully {
		t.Error("spec with constraint block should not be fully representable")
	}
	if !strings.Contains(reason, "constraint") {
		t.Errorf("reason should mention 'constraint': %s", reason)
	}
}

func TestCheckFullyRepresentable_nomadWithUpdate(t *testing.T) {
	job := `job "demo" {
  group "app" {
    update {
      max_parallel = 1
    }
    task "app" {
      driver = "docker"
      config { image = "x:1" }
    }
  }
}
`
	fully, reason := CheckFullyRepresentable("nomad", job, "", nil)
	if fully {
		t.Error("spec with update block should not be fully representable")
	}
	if !strings.Contains(reason, "update") {
		t.Errorf("reason should mention 'update': %s", reason)
	}
}

func TestCheckFullyRepresentable_nomadEmpty(t *testing.T) {
	fully, _ := CheckFullyRepresentable("nomad", "", "", nil)
	if !fully {
		t.Error("empty HCL should be fully representable")
	}
}
