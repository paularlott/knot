// The first label of a space's web host, <user>--<name>--<port>, made to
// fit the 63 characters DNS allows a label, as the server makes it
// (internal/util/hostlabel): a longer label keeps as much of the name as
// fits, then a hyphen and an FNV-1a hash of the whole name.
const maxLabel = 63;
const hashLen = 8;

function nameHash(name) {
  let h = 0x811c9dc5;
  for (const b of new TextEncoder().encode(name.toLowerCase())) {
    h ^= b;
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h.toString(16).padStart(hashLen, "0");
}

export function hostLabel(user, name, port) {
  user = String(user).toLowerCase();
  name = String(name).toLowerCase();
  port = String(port).toLowerCase();
  const full = `${user}--${name}--${port}`;
  if (full.length <= maxLabel) {
    return full;
  }
  const keep = Math.max(1, Math.min(maxLabel - user.length - port.length - 4 - hashLen - 1, name.length));
  return `${user}--${name.slice(0, keep).replace(/-+$/, "")}-${nameHash(name)}--${port}`;
}

export const popup = {
  openTerminal(spaceId) {
    const timestamp = new Date().getTime();
    window.open(`/terminal/${spaceId}`, `spaces_${spaceId}_terminal_${timestamp}`, 'width=800,height=500');
    return false;
  },

  // Open the serial console of a KVM space's VM (login with the owner's
  // knot username and service password).
  openConsole(spaceId) {
    const timestamp = new Date().getTime();
    window.open(`/terminal/${spaceId}/console`, `spaces_${spaceId}_console_${timestamp}`, 'width=800,height=500');
    return false;
  },

  // Open the QEMU display of a KVM space's VM in a noVNC window.
  openVMDisplay(spaceId) {
    const timestamp = new Date().getTime();
    window.open(`/vnc/${spaceId}`, `spaces_${spaceId}_vnc_${timestamp}`, 'width=1024,height=768');
    return false;
  },

  openTerminalTunnel(spaceId) {
    const width = Math.min(screen.width, 900);
    window.open(`/terminal/${spaceId}/vscode-tunnel`, `spaces_${spaceId}_tunnel`, `width=${width},height=400`);
    return false;
  },

  openCodeServer(spaceId) {
    const maxWidth = Math.min(window.innerWidth, 1440);
    const maxHeight = window.innerHeight;
    window.open(`/proxy/spaces/${spaceId}/code-server/`, `spaces_${spaceId}_code_server`, `width=${maxWidth},height=${maxHeight}`);
    return false;
  },

  openVSCodeDev(tunnelName) {
    const maxWidth = Math.min(window.innerWidth, 1440);
    const maxHeight = window.innerHeight;
    window.open(`https://vscode.dev/tunnel/${tunnelName}`, 'vscodedev', `width=${maxWidth},height=${maxHeight}`);
    return false;
  },

  openVNC(spaceId, domain, username, spaceName) {
    const subdomain = `${window.location.protocol}//${hostLabel(username, spaceName, "vnc")}`;
    const maxWidth = Math.min(window.innerWidth, 1440);
    const maxHeight = window.innerHeight;
    const portSuffix = window.location.port ? `:${window.location.port}` : '';

    window.open(domain.replace(/^\*/, subdomain) + portSuffix, `spaces_${spaceId}_vnc`, `width=${maxWidth},height=${maxHeight}`);
    return false;
  },

  openPortWindow(spaceId, domain, username, spaceName, port) {
    const subdomain = `${window.location.protocol}//${hostLabel(username, spaceName, port)}`;
    const portSuffix = window.location.port ? `:${window.location.port}` : '';
    window.open(domain.replace(/^\*/, subdomain) + portSuffix, `spaces_${spaceId}_http_port_${port}`);
    return false;
  },

  openLogWindow(spaceId) {
    window.open(`/logs/${spaceId}`, `spaces_${spaceId}_log`, 'width=800,height=500');
    return false;
  },

};
