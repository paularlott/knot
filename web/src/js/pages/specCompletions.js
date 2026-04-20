const docs = (title, body) => `<b>${title}</b><br/>${body}`;

export const containerSpecCompletions = [
  {
    caption: "image",
    value: 'image: "registry-1.docker.io/library/nginx:latest"',
    meta: "container",
    score: 1000,
    docHTML: docs("image", "Container image to run."),
  },
  {
    caption: "ports",
    value: 'ports:\n  - "8080:80/tcp"',
    meta: "container",
    score: 990,
    docHTML: docs("ports", "Host-to-container port mappings."),
  },
  {
    caption: "volumes",
    value: 'volumes:\n  - "/data:/workspace"',
    meta: "container",
    score: 980,
    docHTML: docs("volumes", "Host path or named volume bindings."),
  },
  {
    caption: "environment",
    value: 'environment:\n  - "KEY=value"',
    meta: "container",
    score: 970,
    docHTML: docs("environment", "Environment variables passed to the container."),
  },
  {
    caption: "command",
    value: 'command:\n  - "sleep"\n  - "infinity"',
    meta: "container",
    score: 960,
    docHTML: docs("command", "Override the image command."),
  },
  {
    caption: "auth",
    value: 'auth:\n  username: "<username>"\n  password: "<password>"',
    meta: "container",
    score: 950,
    docHTML: docs("auth", "Registry authentication for private images."),
  },
  {
    caption: "network",
    value: 'network: "bridge"',
    meta: "container",
    score: 940,
    docHTML: docs("network", "Container network mode."),
  },
  {
    caption: "privileged",
    value: "privileged: false",
    meta: "container",
    score: 930,
    docHTML: docs("privileged", "Run with elevated privileges."),
  },
  {
    caption: "cap_add",
    value: 'cap_add:\n  - "NET_ADMIN"',
    meta: "container",
    score: 920,
    docHTML: docs("cap_add", "Linux capabilities to add."),
  },
  {
    caption: "cap_drop",
    value: 'cap_drop:\n  - "MKNOD"',
    meta: "container",
    score: 910,
    docHTML: docs("cap_drop", "Linux capabilities to drop."),
  },
  {
    caption: "devices",
    value: 'devices:\n  - "/dev/fuse:/dev/fuse"',
    meta: "container",
    score: 900,
    docHTML: docs("devices", "Device mappings from host to container."),
  },
  {
    caption: "add_host",
    value: 'add_host:\n  - "host.docker.internal:192.168.1.10"',
    meta: "container",
    score: 890,
    docHTML: docs("add_host", "Additional host/IP mappings."),
  },
  {
    caption: "dns",
    value: 'dns:\n  - "1.1.1.1"',
    meta: "container",
    score: 880,
    docHTML: docs("dns", "Custom DNS resolver IPs."),
  },
  {
    caption: "dns_search",
    value: 'dns_search:\n  - "internal.example"',
    meta: "container",
    score: 870,
    docHTML: docs("dns_search", "Additional DNS search domains."),
  },
  {
    caption: "memory",
    value: 'memory: "1G"',
    meta: "container",
    score: 860,
    docHTML: docs("memory", "Memory limit in bytes, M, or G."),
  },
  {
    caption: "cpus",
    value: 'cpus: "2"',
    meta: "container",
    score: 850,
    docHTML: docs("cpus", "CPU limit as a decimal string."),
  },
];

export const localVolumeSpecCompletions = [
  {
    caption: "volumes",
    value: "volumes:\n  workspace:\n",
    meta: "volume",
    score: 1000,
    docHTML: docs("volumes", "Map of named local container volumes."),
  },
  {
    caption: "workspace",
    value: "workspace:\n",
    meta: "volume",
    score: 900,
    docHTML: docs("volume name", "A local named volume to create."),
  },
];

export const nomadJobCompletions = [
  {
    caption: "job",
    value: 'job "${{.space.name}}-${{.user.username}}" {\n  datacenters = ["dc1"]\n}\n',
    meta: "nomad",
    score: 1000,
    docHTML: docs("job", "Nomad job block."),
  },
  {
    caption: "group",
    value: 'group "app" {\n  count = 1\n}\n',
    meta: "nomad",
    score: 990,
    docHTML: docs("group", "Nomad task group."),
  },
  {
    caption: "task",
    value: 'task "app" {\n  driver = "docker"\n\n  config {\n    image = "registry-1.docker.io/library/nginx:latest"\n  }\n}\n',
    meta: "nomad",
    score: 980,
    docHTML: docs("task", "Nomad task definition."),
  },
  {
    caption: "volume",
    value: 'volume "data" {\n  type            = "csi"\n  source          = "data-volume"\n  attachment_mode = "file-system"\n  access_mode     = "single-node-writer"\n}\n',
    meta: "nomad",
    score: 970,
    docHTML: docs("volume", "Nomad group volume block."),
  },
  {
    caption: "volume_mount",
    value: 'volume_mount {\n  volume      = "data"\n  destination = "/data"\n}\n',
    meta: "nomad",
    score: 960,
    docHTML: docs("volume_mount", "Nomad task volume mount."),
  },
  {
    caption: "resources",
    value: "resources {\n  cores  = 2\n  memory = 2048\n}\n",
    meta: "nomad",
    score: 950,
    docHTML: docs("resources", "CPU and memory resources."),
  },
  {
    caption: "env",
    value: 'env {\n  TZ = "${{ .user.timezone }}"\n}\n',
    meta: "nomad",
    score: 940,
    docHTML: docs("env", "Environment variables block."),
  },
  {
    caption: "network",
    value: 'network {\n  port "http" {\n    to = 80\n  }\n}\n',
    meta: "nomad",
    score: 930,
    docHTML: docs("network", "Nomad network stanza."),
  },
];

export const nomadVolumeSpecCompletions = [
  {
    caption: "volumes",
    value: "volumes:\n  - name: data\n    type: csi\n    plugin_id: hostpath\n",
    meta: "volume",
    score: 1000,
    docHTML: docs("volumes", "List of Nomad CSI or host volumes."),
  },
  {
    caption: "csi volume",
    value:
      '  - id: "data"\n    name: "data"\n    type: csi\n    plugin_id: "hostpath"\n    capacity_min: 1G\n    capacity_max: 10G\n    mount_options:\n      fs_type: "ext4"\n      mount_flags:\n        - rw\n    capabilities:\n      - access_mode: "single-node-writer"\n        attachment_mode: "file-system"\n',
    meta: "volume",
    score: 990,
    docHTML: docs("CSI volume", "CSI-backed volume definition."),
  },
  {
    caption: "host volume",
    value:
      '  - name: "host-volume"\n    type: host\n    plugin_id: "mkdir"\n    parameters:\n      mode: "0755"\n',
    meta: "volume",
    score: 980,
    docHTML: docs("Host volume", "Nomad host volume definition."),
  },
  {
    caption: "mount_options",
    value: 'mount_options:\n  fs_type: "ext4"\n  mount_flags:\n    - rw',
    meta: "volume",
    score: 970,
    docHTML: docs("mount_options", "Filesystem and mount flags."),
  },
  {
    caption: "capabilities",
    value: 'capabilities:\n  - access_mode: "single-node-writer"\n    attachment_mode: "file-system"',
    meta: "volume",
    score: 960,
    docHTML: docs("capabilities", "CSI attachment and access modes."),
  },
];
