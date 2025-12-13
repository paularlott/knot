window.auditLogComponent = function() {
  return {
    loading: true,
    logs: [],
    currentPage: 0,
    totalPages: 0,

    async init() {
      await this.getAuditLogs();

      // Subscribe to SSE for real-time updates instead of polling
      if (window.sseClient) {
        window.sseClient.subscribe('auditlogs:changed', () => {
          this.getAuditLogs();
        });

        window.sseClient.subscribe('reconnected', () => {
          this.getAuditLogs();
        });
      }
    },

    async getAuditLogs() {
      await fetch(`/api/audit-logs?start=${this.currentPage * 10}`, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((logs) => {
            this.logs = logs;
            this.logs.items.forEach(item => {
              const date = new Date(item.when);
              item.when = date.toLocaleString();
            });

            this.totalPages = Math.ceil(this.logs.count / 10)
            if (this.currentPage >= this.totalPages) {
              this.currentPage = this.totalPages - 1;
            }

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
