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
    value: 'volumes:\n  - "workspace:/workspace"',
    meta: "container",
    score: 980,
    docHTML: docs("volumes", "Host path, managed path, or named volume bindings."),
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
    caption: "paths",
    value: "paths:\n  - workspace\n  - ~/knot-workspace\n",
    meta: "path",
    score: 995,
    docHTML: docs("paths", "List of managed host paths to create for local containers."),
  },
  {
    caption: "workspace",
    value: "workspace:\n",
    meta: "volume",
    score: 900,
    docHTML: docs("volume name", "A local named volume to create."),
  },
  {
    caption: "size",
    value: "size: 20G",
    meta: "volume",
    score: 890,
    docHTML: docs("size", "Volume size (Apple Containers only), e.g. 10G, 512M."),
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
    caption: "paths",
    value: "paths:\n  - /storage/${{ .space.id }}/data\n",
    meta: "path",
    score: 995,
    docHTML: docs("paths", "List of managed host paths to create before Nomad jobs start."),
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

// System + custom template variables available in job and volume templates.
// These resolve at deploy time via the Go template engine using the ${{
// delimiters. Suggested across nomad job, container, and volume editors.
export const templateVariableCompletions = [
  // .space
  {
    caption: "${{ .space.id }}",
    value: "${{ .space.id }}",
    meta: "space",
    score: 1000,
    docHTML: docs("Space ID", "Unique UUID of the space."),
  },
  {
    caption: "${{ .space.name }}",
    value: "${{ .space.name }}",
    meta: "space",
    score: 999,
    docHTML: docs("Space Name", "Name of the space."),
  },
  {
    caption: "${{ .space.stack }}",
    value: "${{ .space.stack }}",
    meta: "space",
    score: 998,
    docHTML: docs("Space Stack", "Stack name the space belongs to (empty if not in a stack)."),
  },
  {
    caption: "${{ .space.stack_prefix }}",
    value: "${{ .space.stack_prefix }}",
    meta: "space",
    score: 997,
    docHTML: docs("Space Stack Prefix", "Prefix used when the space was created as part of a stack. Use to reference sibling containers, e.g. <code>${{ .space.stack_prefix }}-db</code>."),
  },
  {
    caption: "${{ .space.first_boot }}",
    value: "${{ .space.first_boot }}",
    meta: "space",
    score: 996,
    docHTML: docs("Space First Boot", "<code>true</code> on the very first boot of the space, otherwise <code>false</code>."),
  },
  // .template
  {
    caption: "${{ .template.id }}",
    value: "${{ .template.id }}",
    meta: "template",
    score: 950,
    docHTML: docs("Template ID", "UUID of the template the space was created from."),
  },
  {
    caption: "${{ .template.name }}",
    value: "${{ .template.name }}",
    meta: "template",
    score: 949,
    docHTML: docs("Template Name", "Name of the template the space was created from."),
  },
  // .user
  {
    caption: "${{ .user.id }}",
    value: "${{ .user.id }}",
    meta: "user",
    score: 900,
    docHTML: docs("User ID", "UUID of the user who owns the space."),
  },
  {
    caption: "${{ .user.username }}",
    value: "${{ .user.username }}",
    meta: "user",
    score: 899,
    docHTML: docs("Username", "Username of the user who owns the space."),
  },
  {
    caption: "${{ .user.email }}",
    value: "${{ .user.email }}",
    meta: "user",
    score: 898,
    docHTML: docs("Email", "Email address of the user who owns the space."),
  },
  {
    caption: "${{ .user.timezone }}",
    value: "${{ .user.timezone }}",
    meta: "user",
    score: 897,
    docHTML: docs("User Timezone", "Timezone of the user who owns the space."),
  },
  {
    caption: "${{ .user.service_password }}",
    value: "${{ .user.service_password }}",
    meta: "user",
    score: 896,
    docHTML: docs("Service Password", "Auto-generated service password for the user (used for VNC and SSH auth)."),
  },
  // .server
  {
    caption: "${{ .server.url }}",
    value: "${{ .server.url }}",
    meta: "server",
    score: 850,
    docHTML: docs("Server URL", "External URL of the knot server."),
  },
  {
    caption: "${{ .server.agent_endpoint }}",
    value: "${{ .server.agent_endpoint }}",
    meta: "server",
    score: 849,
    docHTML: docs("Agent Endpoint", "Endpoint agents use to connect back to the server."),
  },
  {
    caption: "${{ .server.wildcard_domain }}",
    value: "${{ .server.wildcard_domain }}",
    meta: "server",
    score: 848,
    docHTML: docs("Wildcard Domain", "Wildcard domain used to expose space ports (without the leading <code>*</code>)."),
  },
  {
    caption: "${{ .server.zone }}",
    value: "${{ .server.zone }}",
    meta: "server",
    score: 847,
    docHTML: docs("Server Zone", "Zone name of the knot server."),
  },
  {
    caption: "${{ .server.timezone }}",
    value: "${{ .server.timezone }}",
    meta: "server",
    score: 846,
    docHTML: docs("Server Timezone", "Timezone configured on the knot server."),
  },
  // .nomad
  {
    caption: "${{ .nomad.dc }}",
    value: "${{ .nomad.dc }}",
    meta: "nomad",
    score: 800,
    docHTML: docs("Nomad Datacenter", "Nomad datacenter (from the <code>NOMAD_DC</code> environment variable)."),
  },
  {
    caption: "${{ .nomad.region }}",
    value: "${{ .nomad.region }}",
    meta: "nomad",
    score: 799,
    docHTML: docs("Nomad Region", "Nomad region (from the <code>NOMAD_REGION</code> environment variable)."),
  },
  // .stack — cross-space references to siblings in the same stack. The key is
  // the sibling's stack-definition key (space name with the prefix stripped);
  // complete with <key>.<group>.<name>, e.g. ${{ .stack.db.custom.password }}.
  {
    caption: "${{ .stack.",
    value: "${{ .stack.",
    meta: "stack",
    score: 760,
    docHTML: docs(
      "Stack Sibling Variable",
      "Reference a variable on a sibling space in the same stack. Replace <code>&lt;key&gt;</code> with the sibling's stack key (its name with the stack prefix stripped), then the group and field. Examples: <code>${{ .stack.db.space.id }}</code>, <code>${{ .stack.db.custom.password }}</code>. Keys containing a hyphen use a dotted-safe <code>_</code> alias: <code>${{ .stack.space_1.custom.password }}</code> (equivalently <code>${{ (index .stack \"space-1\").custom.password }}</code>). Only resolves if the sibling space already exists.",
    ),
  },
  // .custom — partial; the user completes the variable name
  {
    caption: "${{ .custom.",
    value: "${{ .custom.",
    meta: "custom",
    score: 750,
    docHTML: docs("Custom Variable", "Inserts the opening of a custom variable. Complete with the variable name and close with <code>}}</code>, e.g. <code>${{ .custom.branch }}</code>."),
  },
];
