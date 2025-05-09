window.clusterInfoComponent = function() {
  return {
    loading: true,
    nodes: [],

    async init() {
      this.getClusterInfo();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getClusterInfo();
      }, 2000);
    },

    async getClusterInfo() {
      const response = await fetch('/api/cluster-info', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.nodes = await response.json();
/*      this.logs.items.forEach(item => {
        const date = new Date(item.when);
        item.when = date.toLocaleString();
      });

      this.totalPages = Math.ceil(this.logs.count / 10)
      if (this.currentPage >= this.totalPages) {
        this.currentPage = this.totalPages - 1;
      }*/

      this.loading = false;
    }
  };
}
