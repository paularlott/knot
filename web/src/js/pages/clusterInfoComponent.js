import { sortable } from '../components/sortable.js';
window.clusterInfoComponent = function() {
  return {
    ...sortable('cluster-info', {
      name: (n) => (n.metadata && n.metadata.hostname) || n.id,
      state: (n) => n.state,
      zone: (n) => n.metadata && n.metadata.zone,
      spaces: { value: (n) => Number((n.metadata && n.metadata.running_spaces) || 0), dir: 'desc' },
    }),
    loading: true,
    nodes: [],

    async init() {
      await this.getClusterInfo();

      // Start a timer to look for updates (skipped while live updates are
      // paused from the header).
      setInterval(async () => {
        if (window.knotLive && window.knotLive.paused) return;
        await this.getClusterInfo();
      }, 2000);
    },

    async getClusterInfo() {
      await fetch('/api/cluster-info', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((nodes) => {
            this.nodes = nodes;
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    }
  };
}
