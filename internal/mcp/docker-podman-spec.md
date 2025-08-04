# Knot Template Job Specification

This document describes how to create templates for Knot development environments using Docker/Podman containers.

## Template Structure

Templates define containerized development environments with the following YAML specification:

```yaml
container_name: <container name>
hostname: <host name>
image: "<container image>"
auth:
  username: <username>
  password: <password>
ports:
  - <host port>:<container port>/<transport>
volumes:
  - <host path>:<container path>
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
  - <host path>:<container path>
dns:
  - <nameserver ip>
add_host:
  - <host name>:<ip>
dns_search:
  - <domain name>
```

## Field Descriptions

### Core Configuration
- **container_name**: Unique container identifier
- **hostname**: Internal container hostname
- **image**: Container image (Docker Hub, private registry, etc.)

### Authentication
- **auth**: Registry credentials for private images
  - **username**: Registry username
  - **password**: Registry password

### Networking & Access
- **ports**: Port mappings `<host>:<container>/<protocol>` (tcp/udp)
- **network**: Network mode (bridge, host, none, container:<name>)
- **dns**: Custom DNS servers
- **add_host**: Custom host entries for /etc/hosts
- **dns_search**: DNS search domains

### Storage & Data
- **volumes**: Volume mounts `<host_path>:<container_path>`

### Runtime Configuration
- **command**: Override default container command (array of strings)
- **environment**: Environment variables `<VAR>=<value>`
- **privileged**: Extended host privileges (use cautiously)

### Security & Capabilities
- **cap_add**: Add Linux capabilities
- **cap_drop**: Remove Linux capabilities
- **devices**: Device mappings `<host_device>:<container_device>`

## Best Practices for AI Template Generation

1. **Development Focus**: Templates should create productive development environments
2. **Port Exposure**: Expose common development ports (3000, 8080, 5000, etc.)
3. **Volume Persistence**: Mount workspace directories for code persistence
4. **Tool Installation**: Include essential development tools in the image
5. **Environment Variables**: Set appropriate development environment variables
6. **Security**: Avoid privileged mode unless absolutely necessary

## Common Template Patterns

### Web Development
```yaml
image: "node:18"
ports:
  - "3000:3000"
  - "8080:8080"
volumes:
  - "/workspace:/app"
environment:
  - "NODE_ENV=development"
```

### Python Development
```yaml
image: "python:3.11"
ports:
  - "8000:8000"
  - "5000:5000"
volumes:
  - "/workspace:/workspace"
environment:
  - "PYTHONPATH=/workspace"
```

### Full Stack Development
```yaml
image: "ubuntu:22.04"
ports:
  - "3000:3000"
  - "8080:8080"
  - "5432:5432"
volumes:
  - "/workspace:/workspace"
command: ["/bin/bash"]
environment:
  - "DEBIAN_FRONTEND=noninteractive"
```