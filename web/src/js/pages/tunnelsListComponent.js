import { sortable } from '../components/sortable.js';
window.tunnelsListComponent = function() {
  return {
    ...sortable('tunnels', {
      name: (t) => t.address || t.name,
    }),
    loading: true,
    tunnels: [],

    async init() {
      await this.getTunnels();

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('tunnels:changed', () => {
          this.getTunnels();
        });

        window.sseClient.subscribe('tunnels:deleted', (payload) => {
          this.tunnels = this.tunnels.filter(x => x.name !== payload?.id);
        });

        window.sseClient.subscribe('reconnected', () => {
          this.getTunnels();
        });
      }
    },

    async getTunnels() {
      await fetch('/api/tunnels', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((tunnels) => {
            this.tunnels = tunnels || [];
            this.loading = false;
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          // e.g. the tunnel server is not enabled: show the empty state
          // instead of spinning forever
          this.tunnels = [];
          this.loading = false;
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
          window.knotError('close the tunnel', response);
        }
      }).catch((err) => {
        // Don't logout on network errors - Safari closes connections aggressively
        window.knotError('close the tunnel', err);
      });
    }
  };
}
