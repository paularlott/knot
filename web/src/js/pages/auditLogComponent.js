window.auditLogComponent = function() {
  return {
    loading: true,
    logs: [],
    currentPage: 0,
    totalPages: 0,

    async init() {
      await this.getAuditLogs();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getAuditLogs();
      }, 3000);
    },

    async getAuditLogs() {
      const response = await fetch(`/api/audit-logs?start=${this.currentPage * 10}`, {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.logs = await response.json();
      this.logs.items.forEach(item => {
        const date = new Date(item.when);
        item.when = date.toLocaleString();
      });

      this.totalPages = Math.ceil(this.logs.count / 10)
      if (this.currentPage >= this.totalPages) {
        this.currentPage = this.totalPages - 1;
      }

      this.loading = false;
    }
  };
}
