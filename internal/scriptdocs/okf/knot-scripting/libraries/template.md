---
type: "API Reference"
title: "knot.template"
description: "Template management functions for space configuration definitions."
resource: "https://getknot.dev/reference/libraries/template/"
tags:
  - "templates"
  - "api"
  - "scripting"
status: stable
generated:
  by: knot/scripts/generate-scripting-docs.mjs
sources:
  - resource: "https://getknot.dev/reference/libraries/template/"
---
# knot.template

The `knot.template` library provides template management functions. Templates define the configuration for creating spaces.

---

## Functions

| Function | Description |
|----------|-------------|
| `list(include_inactive=False, resolve_options=False)` | List templates visible to the current user; with `resolve_options`, handler-backed custom fields have their options resolved as the requesting user |
| `get(template_id, resolve_options=False)` | Get template by ID or name; with `resolve_options`, handler-backed custom fields have their options resolved as the requesting user |
| `field_options(handler_id, query='')` | Resolve a plugin field handler's option keys as the requesting user — the values the space form offers and the API accepts — or `None` when the handler cannot be reached. `query` is passed to the handler as the text typed so far (autocomplete fields) |
| `field_handlers()` | List the field handlers loaded plugins declare (`id`, `label`, `plugin`) |
| `validate(platform, job='', volumes='')` | Validate template job and volume specs without saving. Needs **Manage Templates** |
| `build_spec(platform, spec, original_job='', original_volumes='')` | Build native job/volume text from a unified spec (image, env, ports, storage, resources). The same conversion the [UI spec wizard](https://getknot.dev/docs/configuration/spec-wizard/) uses. Needs **Manage Templates** |
| `nodes(template_id)` | List available nodes for a local-container or KVM template |
| `create(name, job='', description='', platform='', ...)` | Create a new template; returns its ID |
| `update(template_id, ...)` | Update template properties; omitted arguments keep their current values |
| `delete(template_id)` | Delete a template |
| `get_icons()` | Get list of available icons, each with `description`, `source` and `url` |
| `export(template_id)` | Export a template as portable YAML |
| `import_yaml(yaml_text)` | Create or update a template from exported YAML; returns its ID |
| `parse_spec(platform, job, volumes='')` | Convert native job/volume text into a unified spec, as the spec wizard does |
| `base_images()` | Get the spec wizard's base image catalog |
| `refresh_base_images()` | Fetch the base image catalog from its update URL now |
| `capabilities()` | List the Linux capabilities the spec wizard offers |

---

## Usage

```python
import knot.template as template

# List all templates
templates = template.list()
for t in templates:
    print(f"{t['name']}: {t['platform']}")

# Get a template
t = template.get("ubuntu")
print(t['description'])

# Get available icons
icons = template.get_icons()
for icon in icons:
    print(icon["description"], icon["source"], icon["url"])

# Validate a template spec before saving
result = template.validate("docker", job="image: ubuntu:24.04")
print(result["valid"])

# Build a spec from a unified description, then create a template from it
built = template.build_spec("nomad", {
    "image": "nginx:latest",
    "environment": [{"key": "NGINX_HOST", "value": "${{ .space.name }}"}],
    "ports": [{"host_port": 8080, "container_port": 80, "protocol": "tcp"}],
    "memory": "512M",
    "cpus": "1",
    "cpu_type": "cores",
})
template.create("nginx", job=built["job"], volumes=built["volumes"], platform="nomad")
```

### Custom fields

`create()` and `update()` take `custom_fields`, a list of field definitions. The list order is the order the space forms show the fields in, so reordering is just passing the list in a new order (`update()` replaces the whole list).

```python
template.update("nginx", custom_fields=[
    # Required: always shown, and a space can't be created with it blank.
    {"name": "site", "description": "Site name", "required": True},
    # Optional, but shown on the create form with the required fields.
    {"name": "branch", "description": "Git branch", "default": "main", "show_on_create": True},
    # Optional and seldom needed: behind the create form's optional fields toggle.
    {"name": "seed", "description": "Database seed", "type": "textarea", "language": "yaml"},
])
```

Each definition takes `name`, `description`, `type` (`text`, `masked`, `number`, `bool`, `select`, `autocomplete` or `textarea`), `handler` or `options` (select and autocomplete, exactly one), `language` (textarea), `default`, `required` and `show_on_create`. `show_on_create` only affects how the create form is laid out and is ignored on required fields; the edit form always shows every field.

---

## Template Properties

`list()` returns summary objects containing:
- `id` - Template ID
- `name` - Template name
- `description` - Description
- `platform` - Platform: `docker`, `podman`, `apple`, `container` (local container, backend chosen automatically), `nomad`, `kvm`, or `manual`
- `active` - Whether the template is active
- `usage` - Current usage count
- `deployed` - Number of deployed spaces
- `custom_fields` - Custom field definitions (as described for `get()` below)

`get()` returns the full template including all of the above plus:
- `job` - Job definition
- `volumes` - Volume definitions
- `is_managed` - Whether managed by the system
- `compute_units` - Compute units quota
- `storage_units` - Storage units quota
- `hash` - Template hash
- `with_terminal` - Terminal access enabled
- `with_vscode_tunnel` - VS Code tunnel enabled
- `with_code_server` - Code Server enabled
- `with_ssh` - SSH access enabled
- `with_run_command` - Run command enabled
- `with_vnc` - VNC (web desktop) enabled
- `allow_node_migration` - Whether stopped spaces created from this local-container template can be reassigned to another node. Combined with `health_check_auto_restart`, automatic failed-node recovery is available in Knot Pro {{< pro-badge >}}
- `kvm_network_mode` - KVM templates only: `bridged` (static IPs from the template's range) or `nat` (libvirt NAT, DHCP) — see the [VM specification](https://getknot.dev/docs/templates/kvm-templates/vm-spec/)
- `kvm_network_cidr`, `kvm_ip_range_start`, `kvm_ip_range_end`, `kvm_gateway`, `kvm_bridge` - KVM bridged network configuration derived from the template's `network:` block. The IP range is empty when the template uses the network's whole usable address space (the default when no range is set)
- `runtime_available` - Whether any node in the zone currently offers the runtime this template needs (always true for manual and Nomad templates)
- `schedule_enabled` - Schedule enabled
- `auto_start` - Auto-start enabled
- `max_uptime` - Maximum uptime value
- `max_uptime_unit` - Maximum uptime unit
- `idle_timeout` - Idle timeout value; the space is stopped after this long without user activity (terminal input, SSH or web-port traffic, method calls, sustained CPU, and on Pro filesystem writes)
- `idle_timeout_unit` - Idle timeout unit
- `icon_url` - Icon URL
- `groups` - List of group IDs
- `zones` - List of zone names
- `schedule` - List of schedule day dicts (`enabled`, `from`, `to`)
- `custom_fields` - List of custom field dicts: `name`, `description`, `type` (text, masked, number, bool, textarea, select or autocomplete), `required`, `show_on_create` (when set: an optional field shown on the create-space form without opening its optional fields toggle), `default` (when set), and the option source — `options` (the manual list, also the resolved values when `get` ran with `resolve_options=True`) or `handler` (a plugin field handler id). These are the definitions to fill when creating a space: required fields must be set, and select/autocomplete values must be one of the field's options (see [space.create](https://getknot.dev/reference/libraries/space/#create)).
- `health_check_type` - Health check type (`none`, `agent`, `tcp`, `http`, `program`, or `custom`)
- `health_check_config` - Health check target, command, or custom script depending on type
- `health_check_skip_ssl_verify` - Skip TLS verification for HTTP health checks
- `health_check_timeout` - Health check timeout in seconds
- `health_check_interval` - Health check interval in seconds
- `health_check_max_failures` - Number of consecutive failures before the space is considered unhealthy
- `health_check_auto_restart` - Automatically restart when the health check fails. For `agent`, this restarts local-container and Nomad spaces when the agent stops transmitting. Combined with `allow_node_migration`, automatic failed-node recovery is available in Knot Pro {{< pro-badge >}}
- `disable_user_activity` - Whether filesystem user activity collection is disabled for spaces created from this template {{< pro-badge >}}
- `disable_file_storage` - Whether file storage is refused from spaces created from this template
- `disable_user_secrets` - Whether personal secrets are refused from spaces created from this template {{< pro-badge >}}
- `ports` - List of port dicts (`name`, `port`, `protocol`) defining the web ports exposed by spaces created from this template. These are injected as `KNOT_HTTP_PORT`, `KNOT_HTTPS_PORT`, and `KNOT_TCP_PORT` environment variables.

## Create and Update Arguments

`create()` and `update()` take the same keyword arguments. With `create()` unset arguments use the defaults below; with `update()` every argument defaults to `None`, meaning "keep the current value". Lists passed to `update()` replace the whole list.

| Argument | `create()` default | Description |
|----------|-------------------|-------------|
| `name` | (required) | Template name (`update()` only: new name) |
| `job` | `''` | Job definition (container YAML, Nomad HCL or VM spec) |
| `description` | `''` | Description |
| `platform` | `''` | `docker`, `podman`, `apple`, `container`, `nomad`, `kvm` or `manual` |
| `volumes` | `''` | Volume definitions |
| `paths` | `None` | String or list of strings appended to the volume definition as managed `paths` entries |
| `active` | `True` | Whether the template can be used |
| `compute_units` / `storage_units` | `0` | Quota units |
| `with_terminal`, `with_vscode_tunnel`, `with_code_server`, `with_ssh`, `with_run_command`, `with_vnc` | `False` | Enable the corresponding space features |
| `allow_node_migration` | `False` | Allow stopped local-container spaces to be reassigned to another node |
| `schedule_enabled` | `False` | Enable the run schedule |
| `icon_url` | `''` | Icon URL |
| `groups` / `zones` | `None` | Group IDs allowed to use the template / zone restrictions |
| `disable_user_activity` | `False` | Disable filesystem user activity collection {{< pro-badge >}} |
| `disable_file_storage` | `False` | Refuse file storage from the template's spaces |
| `disable_user_secrets` | `False` | Refuse personal secrets from the template's spaces {{< pro-badge >}} |
| `health_check_type` | `'none'` | `none`, `agent`, `tcp`, `http`, `program` or `custom` |
| `health_check_config` | `''` | Target, command or custom script (ignored for `none` and `agent`) |
| `health_check_skip_ssl_verify` | `False` | Skip TLS verification for HTTP checks |
| `health_check_timeout` / `health_check_interval` | `10` / `30` | Seconds |
| `health_check_max_failures` | `3` | Consecutive failures before the space is unhealthy |
| `health_check_auto_restart` | `False` | Restart the space when the health check fails |
| `max_uptime` / `max_uptime_unit` | `0` / `'disabled'` | Maximum uptime before the space is stopped |
| `idle_timeout` / `idle_timeout_unit` | `0` / `'disabled'` | Stop the space after this long without user activity |
| `ports` | `None` | List of `{"name", "port", "protocol"}` dicts; `protocol` is `http`, `https`, `tcp` or `shared` (reachable by every user in the zone through a port forward as `user--space`) |
| `jobs` | `None` | Scheduled jobs seeded into spaces created from the template: list of `{"name", "command", "schedule", "enabled"}` dicts — `schedule` is a 5-field cron expression (empty for manual-only), `enabled` gates automatic firing (see [knot.jobs](../jobs/)) |
| `port_forwards` | `None` | List of `{"local_port", "space", "remote_port"}` dicts seeded into every space created from the template; `space` is a space or pool name you own, or `user--name` for another user's shared port |
| `custom_fields` | `None` | Custom field definitions — see [Custom fields](#custom-fields) |

`get()` does not return `jobs` or `port_forwards`; `update()` preserves them unless you pass new lists.

For `health_check_type="agent"`, no `health_check_config` value is required.

## Validation

`validate(platform, job='', volumes='')` returns:
- `valid` - Whether the specification is valid
- `errors` - List of validation errors with `field` and `message`, plus `line` (the 1-based line in the job or volume text) when the problem can be placed. The key is absent when the specification is valid, so read it with `.get("errors", [])`

## Building Specs

`build_spec(platform, spec, original_job='', original_volumes='')` converts a runtime-agnostic **unified spec** into the platform's native job definition (Nomad HCL or container YAML) plus volume-definition text. It's the same conversion the [UI spec wizard](https://getknot.dev/docs/configuration/spec-wizard/) applies — useful when you want to assemble a template programmatically without hand-writing HCL or YAML.

`spec` is a dict with any of these keys (only `image` is required):

| Key | Type | Description |
|-----|------|-------------|
| `image` | string | Container image to run. |
| `hostname` | string | Container hostname (template variables supported). |
| `name` | string | Container name (`container_name`) or the Nomad job label. Unrelated to `hostname`. |
| `command` | list\[string\] | Command used to start the container. |
| `environment` | list\[{key, value}\] | Environment variables. |
| `ports` | list\[{host_port, container_port, protocol, label}\] | Host-to-container port mappings. `protocol` defaults to `tcp`. |
| `storage` | list\[StorageEntry\] | Mounts — see the spec wizard docs; each entry expands to the bind mount, volume definition, and (Nomad only) the `volume {}` / `volume_mount {}` stanzas. |
| `devices` | list\[{host_path, container_path, cgroup_permissions}\] | Host device mappings. |
| `memory` | string | Memory limit, e.g. `512M`, `1G`. |
| `memory_max` | string | Max memory limit (Nomad only). |
| `cpus` | string | CPU allocation; interpret with `cpu_type`. |
| `cpu_type` | string | Nomad only: `"cores"` (whole cores) or `"mhz"` (default). Ignored by container platforms. |
| `cap_add` / `cap_drop` | list\[string\] | Linux capabilities, e.g. `["CAP_SYS_PTRACE"]`. |
| `network` | string | Network mode. |
| `privileged` | bool | Run privileged. |
| `auth` | {username, password} | Optional registry pull credentials. |
| `templates` | list\[NomadTemplate\] | Nomad `template {}` blocks (Nomad only). |

When `original_job` / `original_volumes` are provided, the server patches the unified spec into them field-by-field, preserving any hand-written content outside the wizard's surface (just like the UI wizard's "Apply"). Pass empty strings (the default) to build from scratch.

`build_spec()` returns:
- `job` - Native job definition text (HCL for Nomad, YAML for container platforms)
- `volumes` - Volume definition text (YAML)

## Parsing Specs

`parse_spec(platform, job, volumes='')` is the reverse of `build_spec()`: it converts a template's native job and volume text into the unified spec, as the spec wizard does when it opens. Needs the **Manage Templates** permission. It returns:
- `wizardable` - Whether the text could be converted
- `reason` - Why not, when `wizardable` is `False`
- `spec` - The unified spec dict, or `None`
- `fully_representable` - Whether the unified spec holds everything in the native text; when `False`, pass the original text to `build_spec()` so what the spec cannot represent is kept
- `advanced_reason` - What the unified spec cannot represent

`base_images()` returns the catalog the wizard's image picker offers: `version`, `manifest_version` (catalog revision, `yyyymmddbb`), `description`, `registry_auth` (whether base image registry credentials are configured) and `images`, each with `name`, `display_name`, `description`, `image`, `icon`, `category`, `tags`, `default_memory`, `default_cpus`, `default_cores`, `recommended` and optional `default_env`, `default_port` and `volumes`.

`refresh_base_images()` fetches the catalog from its update URL now (needs **Manage Templates**, and base image updates enabled with an update URL in the server configuration) and returns `updated`, `active_version` and `update_url`.

`capabilities()` returns the Linux capabilities offered for `cap_add` / `cap_drop`, each with `name` (e.g. `CAP_NET_ADMIN`), `description` and `common`.

## Export and Import

`export(template_id)` returns a template (by name or ID) as portable YAML, with startup and shutdown script IDs replaced by script names. `import_yaml(yaml_text)` takes that YAML on this or another server: a template with the same name is updated in place, otherwise a new one is created; scripts are looked up by name and dropped when missing. Both need the **Manage Templates** permission.

```python
import knot.template as template

yaml_text = template.export("ubuntu-dev")
template_id = template.import_yaml(yaml_text.replace("name: ubuntu-dev", "name: ubuntu-dev-copy"))
```

## Nodes

`nodes(template_id)` returns available placement nodes for node-local templates: local containers (`docker`, `podman`, `apple`, `container`) and KVM. Other templates (Nomad, manual) return an empty list.

Node entries contain:
- `node_id` - Node ID
- `hostname` - Node hostname
- `running_spaces` - Number of currently running spaces on the node
- `total_spaces` - Total spaces assigned to the node
