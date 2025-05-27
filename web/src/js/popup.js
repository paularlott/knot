export const popup = {
  openTerminal(spaceId) {
    const timestamp = new Date().getTime();
    window.open(`/terminal/${spaceId}`, `spaces_${spaceId}_terminal_${timestamp}`, 'width=800,height=500');
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
    const subdomain = `${window.location.protocol}//${username.toLowerCase()}--${spaceName.toLowerCase()}--vnc`;
    const maxWidth = Math.min(window.innerWidth, 1440);
    const maxHeight = window.innerHeight;

    window.open(domain.replace(/^\*/, subdomain), `spaces_${spaceId}_vnc`, `width=${maxWidth},height=${maxHeight}`);
    return false;
  },

  openPortWindow(spaceId, domain, username, spaceName, port) {
    const subdomain = `${window.location.protocol}//${username.toLowerCase()}--${spaceName.toLowerCase()}--${port}`;
    window.open(domain.replace(/^\*/, subdomain), `spaces_${spaceId}_http_port_${port}`);
    return false;
  },

  openLogWindow(spaceId) {
    window.open(`/logs/${spaceId}`, `spaces_${spaceId}_log`, 'width=800,height=500');
    return false;
  }
};
