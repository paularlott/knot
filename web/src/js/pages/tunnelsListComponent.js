window.tunnelsListComponent = function() {
  return {
    loading: true,
    tunnels: [],

    async init() {
      this.getTunnels();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getTunnels();
      }, 3000);
    },

    async getTunnels() {
      const response = await fetch('/api/v1/tunnels', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.tunnels = await response.json();
      this.loading = false;
    }
  };
}
