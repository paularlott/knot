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
    },

    async terminateTunnel(tunnel) {
      let self = this;

      fetch(`/api/v1/tunnels/${tunnel}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if(response.status === 200) {
          self.$dispatch('show-alert', { msg: "Tunnel terminated", type: 'success' });
          self.getTunnels();
        } else {
          self.$dispatch('show-alert', { msg: "Failed to terminate tunnel", type: 'error' });
        }
      });
    }
  };
}
