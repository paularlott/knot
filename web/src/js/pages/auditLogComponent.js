window.auditLogComponent = function() {
  return {
    loading: true,
    logs: [],
    currentPage: 0,
    totalPages: 0,
    exportModal: {
      show: false,
      format: 'csv',
      from: '',
      to: '',
    },

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
    },

    downloadExport() {
      const params = new URLSearchParams({ format: this.exportModal.format });
      if (this.exportModal.from) {
        params.set('from', new Date(this.exportModal.from).toISOString());
      }
      if (this.exportModal.to) {
        // Include the full end day by moving to end-of-day
        const to = new Date(this.exportModal.to);
        to.setHours(23, 59, 59, 999);
        params.set('to', to.toISOString());
      }

      const a = document.createElement('a');
      a.href = `/api/audit-logs/export?${params.toString()}`;
      a.download = `audit-logs.${this.exportModal.format}`;
      a.click();

      this.exportModal.show = false;
    },
  };
}
