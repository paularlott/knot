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
      const response = await fetch('/api/cluster-info', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.nodes = await response.json();
      this.loading = false;
    }
  };
}
