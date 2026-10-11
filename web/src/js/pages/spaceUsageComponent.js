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
    error: "",
    refreshHandle: null,

    async init() {
      await this.refresh();
      this.refreshHandle = setInterval(() => {
        if (!(window.knotLive && window.knotLive.paused)) this.refresh();
      }, 10000);
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
      try {
        const response = await fetch(`/api/spaces/${this.spaceId}/usage/current`);
        if (!response.ok) {
          return;
        }
        this.current = await response.json();
      } catch (_) {
        // The history request reports connection problems.
      }
    },

    showCurrentCards() {
      return !!(this.current?.is_live && this.current?.resource_usage);
    },

    async refreshHistory() {
      let response;
      try {
        response = await fetch(`/api/spaces/${this.spaceId}/usage/history?range=${encodeURIComponent(this.selectedRange)}`);
      } catch (error) {
        this.error = await window.knotErrorMessage("load the usage history", error);
        return;
      }
      if (!response.ok) {
        this.error = await window.knotErrorMessage("load the usage history", response);
        return;
      }

      this.error = "";
      const payload = await response.json();
      this.history = payload.points || [];
      await this.$nextTick();
      this.renderChart();
    },

    // A text summary of the chart for screen readers (the canvas is an image).
    chartSummary() {
      const range = this.selectedRange === "7d" ? "the last 7 days" : "the last hour";
      if (!this.history.length) {
        return `Usage history for ${range}: no data yet.`;
      }
      const series = [
        ["CPU", (p) => p.resource_usage?.cpu_percent || 0],
        ["memory", (p) => this.usagePercent(p.resource_usage?.memory_used_bytes || 0, p.resource_usage?.memory_limit_bytes || 0)],
        ["disk", (p) => this.usagePercent(p.resource_usage?.disk_used_bytes || 0, p.resource_usage?.disk_limit_bytes || 0)],
      ].map(([name, fn]) => {
        const values = this.history.map(fn);
        const avg = values.reduce((a, b) => a + b, 0) / values.length;
        const max = Math.max(...values);
        const last = values[values.length - 1];
        return `${name} averaged ${avg.toFixed(0)}%, peaked at ${max.toFixed(0)}%, latest ${last.toFixed(0)}%`;
      });
      return `Usage history for ${range}, ${this.history.length} points, as a percentage of each limit: ${series.join("; ")}.`;
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

      context.setAttribute("role", "img");
      context.setAttribute("aria-label", this.chartSummary());

      // Each line differs by dash and point shape as well as colour.
      const markers = this.history.length <= 90 ? 3 : 0;

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
              pointStyle: "circle",
              pointRadius: markers,
            },
            {
              label: "Memory %",
              data: memory,
              borderColor: "#10b981",
              backgroundColor: "rgba(16, 185, 129, 0.15)",
              tension: 0.25,
              borderDash: [8, 4],
              pointStyle: "rect",
              pointRadius: markers,
            },
            {
              label: "Disk %",
              data: disk,
              borderColor: "#f59e0b",
              backgroundColor: "rgba(245, 158, 11, 0.15)",
              tension: 0.25,
              borderDash: [2, 3],
              pointStyle: "triangle",
              pointRadius: markers,
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
                // The legend shows each line's point shape, not just colour.
                usePointStyle: true,
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
