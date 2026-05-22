import Chart from "chart.js/auto";

window.spaceUsageComponent = function (spaceId, initialSpaceName) {
  let historyChart = null;

  return {
    loading: true,
    spaceId,
    spaceName: initialSpaceName,
    selectedRange: "1h",
    current: {
      is_live: false,
      resource_usage: {
        cpu_percent: 0,
        memory_used_bytes: 0,
        memory_limit_bytes: 0,
        disk_used_bytes: 0,
        disk_limit_bytes: 0,
      },
    },
    history: [],
    refreshHandle: null,

    async init() {
      await this.refresh();
      this.refreshHandle = setInterval(() => this.refresh(), 10000);
    },

    destroy() {
      if (this.refreshHandle) {
        clearInterval(this.refreshHandle);
        this.refreshHandle = null;
      }

      if (historyChart) {
        historyChart.destroy();
        historyChart = null;
      }
    },

    async refresh() {
      await Promise.all([this.refreshCurrent(), this.refreshHistory()]);
      this.loading = false;
    },

    async setRange(rangeName) {
      this.selectedRange = rangeName;
      await this.refreshHistory();
    },

    async refreshCurrent() {
      const response = await fetch(`/api/spaces/${this.spaceId}/usage/current`);
      if (!response.ok) {
        return;
      }

      this.current = await response.json();
    },

    showCurrentCards() {
      return !!(this.current?.is_live && this.current?.resource_usage);
    },

    async refreshHistory() {
      const response = await fetch(`/api/spaces/${this.spaceId}/usage/history?range=${encodeURIComponent(this.selectedRange)}`);
      if (!response.ok) {
        return;
      }

      const payload = await response.json();
      this.history = payload.points || [];
      await this.$nextTick();
      this.renderChart();
    },

    renderChart() {
      const context = this.$refs.historyChart;
      if (!context) {
        return;
      }

      const textColor = document.documentElement.classList.contains("dark")
        ? "#e5e7eb"
        : "#374151";

      const labels = this.history.map((point) =>
        this.formatBucketLabel(point.bucket_start),
      );
      const cpu = this.history.map(
        (point) => point.resource_usage?.cpu_percent || 0,
      );
      const memory = this.history.map((point) =>
        this.usagePercent(
          point.resource_usage?.memory_used_bytes || 0,
          point.resource_usage?.memory_limit_bytes || 0,
        ),
      );
      const disk = this.history.map((point) =>
        this.usagePercent(
          point.resource_usage?.disk_used_bytes || 0,
          point.resource_usage?.disk_limit_bytes || 0,
        ),
      );

      if (historyChart) {
        historyChart.destroy();
      }

      historyChart = new Chart(context, {
        type: "line",
        data: {
          labels,
          datasets: [
            {
              label: "CPU %",
              data: cpu,
              borderColor: "#3b82f6",
              backgroundColor: "rgba(59, 130, 246, 0.15)",
              tension: 0.25,
              pointRadius: 0,
            },
            {
              label: "Memory %",
              data: memory,
              borderColor: "#10b981",
              backgroundColor: "rgba(16, 185, 129, 0.15)",
              tension: 0.25,
              pointRadius: 0,
            },
            {
              label: "Disk %",
              data: disk,
              borderColor: "#f59e0b",
              backgroundColor: "rgba(245, 158, 11, 0.15)",
              tension: 0.25,
              pointRadius: 0,
            },
          ],
        },
        options: {
          animation: false,
          responsive: true,
          maintainAspectRatio: false,
          interaction: {
            mode: "index",
            intersect: false,
          },
          scales: {
            x: {
              ticks: {
                color: textColor,
                maxTicksLimit: 8,
              },
              grid: {
                color: "rgba(148, 163, 184, 0.15)",
              },
            },
            y: {
              beginAtZero: true,
              max: 100,
              ticks: {
                color: textColor,
                callback: (value) => `${value}%`,
              },
              grid: {
                color: "rgba(148, 163, 184, 0.15)",
              },
            },
          },
          plugins: {
            legend: {
              labels: {
                color: textColor,
              },
            },
          },
        },
      });
    },

    usagePercent(used, limit) {
      if (!limit) {
        return 0;
      }

      return Math.max(0, Math.min(100, (used / limit) * 100));
    },

    formatPercent(value) {
      return `${Number(value || 0).toFixed(1)}%`;
    },

    formatBytes(value) {
      if (!value) {
        return "0 B";
      }

      const units = ["B", "KB", "MB", "GB", "TB"];
      let current = value;
      let unit = 0;
      while (current >= 1024 && unit < units.length - 1) {
        current /= 1024;
        unit++;
      }

      return `${current.toFixed(current >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`;
    },

    formatUsage(used, limit) {
      if (!limit) {
        return this.formatBytes(used);
      }

      return `${this.formatBytes(used)} / ${this.formatBytes(limit)}`;
    },

    formatBucketLabel(value) {
      const date = new Date(value);
      if (this.selectedRange === "7d") {
        return date.toLocaleDateString();
      }

      return date.toLocaleTimeString([], {
        hour: "numeric",
        minute: "2-digit",
      });
    },
  };
};
