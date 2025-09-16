---
title: knot Podman Job Template Specification for AI Assistants
---

# knot Podman Job Template Specification for AI Assistants
This document specifies the format for creating knot job templates for Podman containers. Your primary goal is to assist users in generating the necessary YAML configurations to run applications in Podman containers managed by knot.

## Core Concepts: A Two-Part Template
A complete knot template consists of **two separate YAML** structures that work together:

1. **The Job Specification**: This YAML data defines the container itself—its image, environment variables, port mappings, and how it uses volumes.
2. **The Volume Definitions**: This YAML data declares the named volumes that will be used by the Job. These volumes provide persistent storage.

You must generate **both** of these for any template that requires persistent volumes.

## Part 1: The Job Specification
This is the main YAML data that defines the container's runtime configuration.

### Job Specification Structure

```yaml
container_name: <container_name>
hostname: <hostname>
image: "<image>"
auth:
  username: <username>
  password: <password>
ports:
  - <host_port>:<container_port>/<protocol>
volumes:
  - <host_path>:<container_path>
command: [
  "<1>",
  "<2>"
]
privileged: <true | false>
network: <network mode>
environment:
  - "<variable>=<value>"
cap_add:
  - <cap>
cap_drop:
  - <cap>
devices:
  - <host_device>:<container_device>
dns:
  - <nameserver_ip>
add_host:
  - <hostname>:<ip>
dns_search:
  - <domain_name>
```

#### Field Descriptions

##### Core Configuration
- **container_name**: Unique container identifier, must be present and if not specified should be set to `${{ .user.username }}-${{ .space.name }}`
- **hostname**: Internal container hostname, must be present and if not given should be set to `${{ .space.name }}`
- **image**: Container image, this must be present. NOTE: if the container image doesn't include a domain name then prepend registry-1.docker.io

##### Authentication
- **auth**: Registry credentials for private images
  - **username**: Registry username
  - **password**: Registry password

##### Networking & Access
- **ports**: Port mappings `<host>:<container>/<protocol>` (tcp/udp)
- **network**: Network mode (bridge, host, none, container:<name>)
- **dns**: Custom DNS servers
- **add_host**: Custom host entries for /etc/hosts
- **dns_search**: DNS search domains

For most templates these will not be required and can be excluded.

##### Storage & Data
- **volumes**: Volume mounts `<host_path>:<container_path>`

Where the `host_path` is to be a location from the host it must be prefixed with a `/`.

Where the `host_path` is a volume is needs to start with the space ID `${{.space.id}}` e.g. `${{.space.id}}-home:/home`. Volumes also need to be added to the templates volume definition, the following example references two volumes, one for home and one for data:

```yaml
volumes:
  ${{.space.id}}-home:
  ${{.space.id}}-data:
```

##### Runtime Configuration
- **command**: Override default container command (array of strings)
- **environment**: Environment variables `<VAR>=<value>`
- **privileged**: Extended host privileges (use cautiously)

For most templates these will not be required and can be excluded.

##### Security & Capabilities
- **cap_add**: Add Linux capabilities
- **cap_drop**: Remove Linux capabilities
- **devices**: Device mappings `<host_device>:<container_device>`

For Podman containers you can include `CAP_NET_RAW` to allow ping to work e.g.

```yaml
cap_add:
  - CAP_NET_RAW
```

### Naming Ports

The environment variables `KNOT_HTTP_PORT` and `KNOT_TCP_PORT` can be used to name ports within the container, to name Web on port 80 and Email on port 8025 you would use `KNOT_HTTP_PORT=80=Web,8025=Email`.

## Part 2: The Volume Definitions
This is a separate YAML structure that declares the named volumes.

### Volume Definition Structure

```yaml
volumes:
  <volume_name_1>:
  <volume_name_2>:
```

- **Naming Convention (Strict)**: Volume names **must** follow the format `${{.space.id}}-<purpose>`.

## Handling Volumes: A Two-Step Process
When a user asks to "mount a volume," you must follow this procedure. Do not ask for the volume's "content."

### Step 1: Define the Volume in the Volume Definitions

- Create a volume name (e.g., `${{.space.id}}-home`).
- Add it to the `volumes:` block in the Volume Definitions YAML.

### Step 2: Mount the Volume in the Job Specification
Add an entry to the volumes list in the Job Specification YAML using the format `<volume_name>:<container_path>` (e.g., `${{.space.id}}-home:/home`).

## Template Variables
The following system variables are available and are substituted at runtime. Use them with the `${{.variable_name}}` syntax.

| **Name**               | **Description**                                                                 |
|------------------------|---------------------------------------------------------------------------------|
| `space.id`             | The UUID of the space                                                          |
| `space.name`           | The name of the space                                                          |
| `space.first_boot`     | Flags if this is the first boot of the space                                   |
| `template.id`          | The UUID of the template used to create the space                              |
| `template.name`        | The name of the template used to create the space                              |
| `user.id`              | The UUID of the user running the space                                         |
| `user.timezone`        | The timezone of the user                                                       |
| `user.username`        | The username of the user running the space                                     |
| `user.email`           | The user's email address                                                       |
| `user.service_password`| Service password for the user                                                  |
| `server.url`           | The URL of the **knot** server                                                 |
| `server.agent_endpoint`| The endpoint agents should use to connect to the server                        |
| `server.wildcard_domain`| The wildcard domain without the leading `*`                                   |
| `server.zone`          | The server zone string                                                         |
| `server.timezone`      | The server timezone                                                            |

User-defined variables are referenced as `${{.var.my_variable}}`.

## Best Practices & Rules for AI Generation

1. **Generate Both Outputs**: For request involving volumes, provide both the **Job Specification YAML** and the **Volume Definitions YAML**. If no volumes are requested, provide only the Job Specification.
2. **Handle Podman Platform**: Podman templates automatically include the `cap_add` section with `CAP_NET_RAW` for better network utility support (like ping).
3. **Default to Named Volumes**: Always use the two-step named volume process unless the user explicitly provides a host path starting with /.
4. **Security First**: Avoid privileged: true. Use it only when essential and explicitly requested.
5. **Confirm with User**: After generating the YAML, always present it to the user and ask for confirmation before finalizing.
6. **Mandatory Environment Variables**: Every Job Specification MUST include the following four KNOT_ environment variables.
  ```yaml
  environment:
    - KNOT_USER=${{.user.username}}
    - KNOT_SERVER=${{.server.url}}
    - KNOT_AGENT_ENDPOINT=${{.server.agent_endpoint}}
    - KNOT_SPACEID=${{.space.id}}
  ```

## Podman-Specific Notes

- Podman containers include `CAP_NET_RAW` by default for network utility support
- Use rootless container execution when possible
- Registry authentication is handled through the `auth` block