window.clusterInfoComponent = function() {
  return {
    loading: true,
    nodes: [],

    async init() {
      await this.getClusterInfo();

      // Start a timer to look for updates
      setInterval(async () => {
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
