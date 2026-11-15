---
type: "API Reference"
title: "knot.volume"
description: "Volume management functions for the volume definitions attached to spaces."
resource: "https://getknot.dev/reference/libraries/volume/"
tags:
  - "storage"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/volume/"
---
# knot.volume

The `knot.volume` library provides volume management functions. Volumes are standalone volume definitions that spaces can mount: on local container platforms (`docker`, `podman`, `apple`, `container`) a named container volume, and on Nomad a CSI or host volume. CSI volumes exist only on Nomad. See [Volumes](https://getknot.dev/docs/volumes/) for the definition formats.

---

## Functions

| Function | Description |
|----------|-------------|
| `list()` | List all volumes |
| `get(volume_id)` | Get volume by ID or name |
| `nodes(platform)` | List available nodes for a platform |
| `validate(platform, definition)` | Validate a volume definition without saving |
| `create(name, definition, platform='', node_id='')` | Create a new volume |
| `update(volume_id, name=None, definition=None, platform=None, node_id=None)` | Update volume properties; `None` keeps the current value. Returns `True` |
| `delete(volume_id)` | Delete a volume |
| `start(volume_id)` | Start a volume |
| `stop(volume_id)` | Stop a volume |
| `is_running(volume_id)` | Check if volume is running |

---

## Usage

```python
import knot.volume as volume

# List all volumes
volumes = volume.list()
for v in volumes:
    status = "active" if v['active'] else "inactive"
    print(f"{v['name']}: {status} ({v['platform']}) node={v['node_hostname']}")

# List available nodes for docker platform
nodes = volume.nodes("docker")
for n in nodes:
    print(f"{n['hostname']} ({n['node_id']})")

# A local-container volume definition: a volumes map with exactly one volume
definition = "volumes:\n  my_data:\n"

# Validate a volume definition first
result = volume.validate("docker", definition)
print(result["valid"])

# Create a volume on a specific node
volume_id = volume.create(
    "my-volume",
    definition=definition,
    platform="docker",
    node_id=nodes[0]['node_id']
)

# Create a volume with auto node selection
volume_id = volume.create(
    "other-volume",
    definition="volumes:\n  other_data:\n",
    platform="docker"
)

# Start a volume
volume.start("my-volume")

# Check if running
if volume.is_running("my-volume"):
    print("Volume is active")

# Stop a volume
volume.stop("my-volume")
```

---

## Volume Properties

Volumes contain:
- `id` - Volume ID
- `name` - Volume name
- `definition` - Volume definition YAML (from `get()` only)
- `active` - Whether the volume is active/running
- `zone` - Zone where volume is running
- `node_id` - ID of the node the volume is assigned to
- `node_hostname` - Hostname of the node the volume is assigned to
- `platform` - Platform type (docker, podman, nomad, etc.)

---

## Node Properties

Nodes returned by `nodes()` contain:
- `node_id` - Node ID
- `hostname` - Node hostname

---

## Validation

`validate(platform, definition)` returns:
- `valid` - Whether the volume definition is valid
- `errors` - List of validation errors with `field` and `message`, plus `line` (the 1-based line in the definition) when the problem can be placed. The key is absent when the definition is valid, so read it with `.get("errors", [])`

---

## Definition Format

The volume definition is a YAML string, and its shape depends on the platform.

On the local container platforms (`docker`, `podman`, `apple`, `container`) it is a top-level `volumes` map holding exactly one volume (Docker and Podman also accept a `driver` and `driver_opts`, e.g. for NFS; Apple accepts a `size`):

```yaml
volumes:
  my_data:
```

On Nomad it is a `volumes` list with exactly one CSI (`plugin_id`, capacities, mount options) or host volume:

```yaml
volumes:
  - id: "test_home"
    name: "test_home"
    plugin_id: "hostpath"
    capacity_min: 10G
    capacity_max: 10G
```

See [Volumes](https://getknot.dev/docs/volumes/) for the full formats.
