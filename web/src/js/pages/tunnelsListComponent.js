window.tunnelsListComponent = function() {
  return {
    loading: true,
    tunnels: [],

    async init() {
      await this.getTunnels();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getTunnels();
      }, 3000);
    },

    async getTunnels() {
      await fetch('/api/tunnels', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((tunnels) => {
            this.tunnels = tunnels;
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },

    async terminateTunnel(tunnel) {
      const self = this;

      await fetch(`/api/tunnels/${tunnel}`, {
        method: 'DELETE',
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if(response.status === 200) {
          self.$dispatch('show-alert', { msg: "Tunnel terminated", type: 'success' });
          self.getTunnels();
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          self.$dispatch('show-alert', { msg: "Failed to terminate tunnel", type: 'error' });
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    }
  };
}
