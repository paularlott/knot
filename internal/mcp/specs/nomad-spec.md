# Nomad Job Specification for Knot Templates

## Overview
This specification defines the structure and requirements for Nomad job definitions used in Knot templates. Nomad jobs define how applications are deployed and managed within a Nomad cluster.

## Basic Job Structure

```hcl
job "example-job" {
  datacenters = ["dc1"]
  type = "service"

  group "app" {
    count = 1

    network {
      port "http" {
        to = 8080
      }
    }

    task "web" {
      driver = "docker"

      config {
        image = "nginx:latest"
        ports = ["http"]
      }

      resources {
        cpu    = 500
        memory = 256
      }
    }
  }
}
```

## Required Fields

### Job Level
- **job**: Job name (must be unique)
- **datacenters**: List of datacenters where the job can run
- **type**: Job type (typically "service" for long-running applications)

### Group Level
- **group**: Logical grouping of tasks
- **count**: Number of instances to run

### Task Level
- **task**: Individual task definition
- **driver**: Task driver (docker, exec, java, etc.)
- **config**: Driver-specific configuration
- **resources**: Resource requirements (CPU, memory)

## Common Drivers

### Docker Driver
```hcl
task "app" {
  driver = "docker"

  config {
    image = "myapp:latest"
    ports = ["http"]
    volumes = [
      "/host/path:/container/path"
    ]
  }
}
```

### Exec Driver
```hcl
task "app" {
  driver = "exec"

  config {
    command = "/usr/bin/myapp"
    args = ["--config", "/etc/myapp.conf"]
  }
}
```

## Networking

### Port Configuration
```hcl
network {
  port "http" {
    static = 8080  # Static port
  }
  port "api" {
    to = 3000      # Dynamic port mapped to container port 3000
  }
}
```

## Resource Constraints

```hcl
resources {
  cpu    = 500    # MHz
  memory = 512    # MB

  device "nvidia/gpu" {
    count = 1
  }
}
```

## Environment Variables

```hcl
env {
  DATABASE_URL = "postgres://localhost/mydb"
  LOG_LEVEL = "info"
}
```

## Health Checks

```hcl
service {
  name = "web-app"
  port = "http"

  check {
    type     = "http"
    path     = "/health"
    interval = "10s"
    timeout  = "3s"
  }
}
```

## Volumes and Storage

```hcl
volume "data" {
  type      = "host"
  source    = "myvolume"
  read_only = false
}

task "app" {
  volume_mount {
    volume      = "data"
    destination = "/data"
  }
}
```

## Template Variables

Knot provides template variables that can be used in job specifications:

- **{{ .Space.Name }}**: Space name
- **{{ .Space.Id }}**: Space ID
- **{{ .User.Username }}**: Username
- **{{ .User.Email }}**: User email
- **{{ .Template.Name }}**: Template name

Example usage:

```hcl
env {
  SPACE_NAME = "{{ .Space.Name }}"
  USER_NAME = "{{ .User.Username }}"
}
```

## Best Practices

1. **Resource Limits**: Always specify appropriate CPU and memory limits
2. **Health Checks**: Include health checks for service discovery
3. **Logging**: Configure proper logging drivers
4. **Security**: Use least privilege principles
5. **Networking**: Use dynamic ports when possible
6. **Volumes**: Use CSI volumes for persistent storage

## Complete Example

```hcl
job "web-app" {
  datacenters = ["dc1"]
  type = "service"

  group "web" {
    count = 1

    network {
      port "http" {
        to = 8080
      }
    }

    volume "data" {
      type   = "csi"
      source = "web-data"
    }

    task "app" {
      driver = "docker"

      config {
        image = "nginx:alpine"
        ports = ["http"]
      }

      volume_mount {
        volume      = "data"
        destination = "/usr/share/nginx/html"
      }

      env {
        SPACE_NAME = "{{ .Space.Name }}"
        USER_NAME = "{{ .User.Username }}"
      }

      resources {
        cpu    = 500
        memory = 256
      }

      service {
        name = "web-app"
        port = "http"

        check {
          type     = "http"
          path     = "/"
          interval = "10s"
          timeout  = "3s"
        }
      }
    }
  }
}
```

For more detailed information, refer to the official Nomad documentation at https://www.nomadproject.io/docs/job-specification
