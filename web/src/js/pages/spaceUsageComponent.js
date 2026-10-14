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
      const percent = (v) => `${v.toFixed(0)}%`;
      const bytes = (v) => this.formatBytes(v);
      const diskBytes = !this.diskHasLimit();
      const series = [
        ["CPU", (p) => p.resource_usage?.cpu_percent || 0, percent],
        ["memory", (p) => this.usagePercent(p.resource_usage?.memory_used_bytes || 0, p.resource_usage?.memory_limit_bytes || 0), percent],
        diskBytes
          ? ["disk used", (p) => p.resource_usage?.disk_used_bytes || 0, bytes]
          : ["disk", (p) => this.usagePercent(p.resource_usage?.disk_used_bytes || 0, p.resource_usage?.disk_limit_bytes || 0), percent],
      ].map(([name, fn, format]) => {
        const values = this.history.map(fn);
        const avg = values.reduce((a, b) => a + b, 0) / values.length;
        const max = Math.max(...values);
        const last = values[values.length - 1];
        return `${name} averaged ${format(avg)}, peaked at ${format(max)}, latest ${format(last)}`;
      });
      const scale = diskBytes ? "CPU and memory as a percentage of each limit, disk as the space used" : "as a percentage of each limit";
      return `Usage history for ${range}, ${this.history.length} points, ${scale}: ${series.join("; ")}.`;
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
      // With no disk limit — a space whose home is on the host's disk —
      // there's nothing to take a percentage of, so disk is drawn as the
      // space used, on its own axis.
      const diskBytes = !this.diskHasLimit();
      const disk = this.history.map((point) =>
        diskBytes
          ? point.resource_usage?.disk_used_bytes || 0
          : this.usagePercent(
              point.resource_usage?.disk_used_bytes || 0,
              point.resource_usage?.disk_limit_bytes || 0,
            ),
      );
      const diskAxis = this.byteAxis(diskBytes ? Math.max(0, ...disk) : 0);

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
              label: diskBytes ? "Disk used" : "Disk %",
              data: disk,
              yAxisID: diskBytes ? "yDisk" : "y",
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
            yDisk: {
              display: diskBytes,
              position: "right",
              beginAtZero: true,
              min: 0,
              max: diskAxis.max,
              ticks: {
                color: textColor,
                stepSize: diskAxis.step,
                callback: (value) => `${Number((value / diskAxis.divisor).toFixed(1))} ${diskAxis.unit}`,
              },
              grid: {
                drawOnChartArea: false,
              },
            },
          },
          plugins: {
            tooltip: {
              callbacks: {
                label: (item) => {
                  const point = this.history[item.dataIndex]?.resource_usage || {};
                  if (item.dataset.yAxisID === "yDisk") {
                    return `Disk used: ${this.formatBytes(item.parsed.y)}`;
                  }
                  if (item.datasetIndex === 2) {
                    return `Disk: ${item.parsed.y.toFixed(1)}% (${this.formatUsage(point.disk_used_bytes || 0, point.disk_limit_bytes || 0)})`;
                  }
                  if (item.datasetIndex === 1) {
                    return `Memory: ${item.parsed.y.toFixed(1)}% (${this.formatUsage(point.memory_used_bytes || 0, point.memory_limit_bytes || 0)})`;
                  }
                  return `CPU: ${item.parsed.y.toFixed(1)}%`;
                },
              },
            },
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

    // A byte axis with round steps in a unit that suits the largest value:
    // KB, MB, GB or TB, with a step of 1, 2, 2.5 or 5 times a power of ten
    // of it, giving four to six steps, and the top rounded up to a step.
    byteAxis(maxValue) {
      const units = [["KB", 1024], ["MB", 1024 ** 2], ["GB", 1024 ** 3], ["TB", 1024 ** 4]];
      let [unit, divisor] = units[1];
      for (const [name, size] of units) {
        if (maxValue >= size) {
          [unit, divisor] = [name, size];
        }
      }
      const top = Math.max(maxValue / divisor, 1);
      const rough = top / 5;
      const magnitude = 10 ** Math.floor(Math.log10(rough));
      const factor = [1, 2, 2.5, 5, 10].find((f) => f * magnitude >= rough);
      const step = factor * magnitude;
      return { unit, divisor, step: step * divisor, max: Math.ceil(top / step) * step * divisor };
    },

    // Whether the space's disk has a limit to take a percentage of, going by
    // the latest point: points from before a change, such as older agents
    // that reported the host's disk as the limit, mustn't flatten the newer
    // ones. Every point carries the bytes used, so all draw on a byte axis.
    diskHasLimit() {
      const latest = this.history[this.history.length - 1];
      return (latest?.resource_usage?.disk_limit_bytes || 0) > 0;
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

    // CPU as a share of what the space may use, with the cores in use when
    // the agent reports them: "6.7% · 1.0 cores".
    formatCpu(usage) {
      const percent = this.formatPercent(usage?.cpu_percent || 0);
      if (typeof usage?.cpu_cores !== "number") {
        return percent;
      }
      const cores = usage.cpu_cores;
      return `${percent} · ${cores >= 1 ? cores.toFixed(1) : cores.toFixed(2)} cores`;
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
