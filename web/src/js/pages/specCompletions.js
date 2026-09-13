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

export const kvmSpecCompletions = [
  {
    caption: "image",
    value: "image: ubuntu-24.04",
    meta: "kvm",
    score: 1000,
    docHTML: docs("image", "Cloud-init capable qcow2 image: a bare name resolved in the node's cloud images directory (.qcow2 appended), a URL (downloaded and cached), or an absolute path."),
  },
  {
    caption: "network",
    value:
      "network:\n  mode: bridged\n  bridge: br0\n  cidr: 192.0.2.0/24",
    meta: "kvm",
    score: 990,
    docHTML: docs("network", "VM networking — required. Bridged attaches to a host bridge with static IPs from the range; NAT attaches to a libvirt network and the VM DHCPs (no addressing fields)."),
  },
  {
    caption: "network: nat",
    value: 'network:\n  mode: nat\n  bridge: "default"',
    meta: "kvm",
    score: 989,
    docHTML: docs("network: nat", "NAT mode: the VM attaches to a libvirt network (default 'default') and gets its address by DHCP. cidr, ip_range_*, and gateway must not be set."),
  },
  {
    caption: "cidr",
    value: "cidr: 192.0.2.0/24",
    meta: "kvm",
    score: 980,
    docHTML: docs("cidr", "Bridged only. The IPv4 network the VMs live on; supplies the address prefix and anchors the gateway/broadcast defaults."),
  },
  {
    caption: "ip_range_start",
    value: "ip_range_start: 192.0.2.10",
    meta: "kvm",
    score: 979,
    docHTML: docs("ip_range_start", "Bridged only, optional. With ip_range_end: the slice of the subnet spaces pick their IP from. Omit both to use the network's whole usable address space (gateway never handed out). Both or neither."),
  },
  {
    caption: "ip_range_end",
    value: "ip_range_end: 192.0.2.100",
    meta: "kvm",
    score: 978,
    docHTML: docs("ip_range_end", "Bridged only, optional. See ip_range_start — both or neither."),
  },
  {
    caption: "gateway",
    value: "gateway: 192.0.2.1",
    meta: "kvm",
    score: 977,
    docHTML: docs("gateway", "Bridged only, optional. Defaults to the network's first usable address."),
  },
  {
    caption: "bridge",
    value: "bridge: br0",
    meta: "kvm",
    score: 976,
    docHTML: docs("bridge", "Bridged: host Linux bridge (default br0) with the physical NIC enslaved. NAT: libvirt network name (default 'default')."),
  },
  {
    caption: "devices",
    value: 'devices:\n  - pci_0000_01_00_0',
    meta: "kvm",
    score: 970,
    docHTML: docs("devices", "Host device passthrough: PCI address (pci_0000_01_00_0), USB pair (usb_002_003) or vendor:product (0x8086:0x1234). PCI needs IOMMU + vfio on the node."),
  },
  {
    caption: "memory",
    value: "memory: 2G",
    meta: "kvm",
    score: 960,
    docHTML: docs("memory", "VM memory, e.g. 2G. Defaults to 512M."),
  },
  {
    caption: "cpus",
    value: "cpus: 2",
    meta: "kvm",
    score: 950,
    docHTML: docs("cpus", "vCPU count. Defaults to 1."),
  },
  {
    caption: "disk",
    value: "disk: 20G",
    meta: "kvm",
    score: 940,
    docHTML: docs("disk", "Caps the VM disk's virtual size. Empty keeps the base image's size; the disk persists across stop/start."),
  },
  {
    caption: "name",
    value: "name: ${{ .user.username }}-${{ .space.name }}",
    meta: "kvm",
    score: 930,
    docHTML: docs("name", "libvirt domain name. Defaults to <username>-<spacename>."),
  },
  {
    caption: "hostname",
    value: "hostname: ${{ .space.name }}",
    meta: "kvm",
    score: 920,
    docHTML: docs("hostname", "VM hostname. Defaults to the space name."),
  },
  {
    caption: "environment",
    value: 'environment:\n  - "KEY=value"',
    meta: "kvm",
    score: 910,
    docHTML: docs("environment", "Written into the agent's environment file inside the VM; values are single-quoted automatically."),
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
  {
    caption: "cap_add",
    value: 'cap_add = ["net_admin"]',
    meta: "nomad",
    score: 920,
    docHTML: docs(
      "cap_add",
      "Linux capabilities to grant, docker driver config block. Subject to the client's allow_caps setting.",
    ),
  },
  {
    caption: "cap_drop",
    value: 'cap_drop = ["mknod"]',
    meta: "nomad",
    score: 910,
    docHTML: docs("cap_drop", "Linux capabilities to revoke, docker driver config block."),
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
