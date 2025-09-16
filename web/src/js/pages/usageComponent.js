import Chart from 'chart.js/auto';

function initSpacesChart(ident, textColor) {
  // Create the space usage chart
  const chartConfig = {
    type: 'doughnut',
    data: {
      labels: ['Running', 'Stopped', 'Available'],
      datasets: [
        {
          data: [ 0, 0, 0 ],
          backgroundColor: [
            '#3b82f6', // Tailwind blue-500
            '#8b5cf6', // Tailwind purple-500
            '#94a3b8' // Tailwind slate-400
          ],
          hoverOffset: 4,
        },
        // Background dataset (will show when main dataset is all zeros)
        {
          data: [ 1, 1 ],
          backgroundColor: ['#e2e8f0'], // Tailwind slate-200
          borderWidth: 0,
          hoverOffset: 0,
          weight: 0.1, // Make this less visually dominant
        },
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: 'bottom',
          labels: {
            color: textColor,
          },
        },
        title: {
          display: true,
          text: 'Space Usage',
          color: textColor,
          font: { size: 16, weight: 'normal', },
        },
        tooltip: {
          filter(tooltipItem) {
            // Only show tooltips for the main dataset (index 0)
            return tooltipItem.datasetIndex === 0;
          }
        },
      }
    },
  };

  const chart = new Chart(
    document.getElementById(ident),
    chartConfig
  );

  return chart;
}

function initTunnelChart(ident, textColor) {
  // Create the tunnel usage chart
  const chartConfig = {
    type: 'doughnut',
    data: {
      labels: ['Used', 'Available'],
      datasets: [
        {
          data: [ 0, 0 ],
          backgroundColor: [
            '#3b82f6', // Tailwind blue-500
            '#94a3b8' // Tailwind slate-400
          ],
          hoverOffset: 4,
        },
        // Background dataset (will show when main dataset is all zeros)
        {
          data: [ 1, 1 ],
          backgroundColor: ['#e2e8f0'], // Tailwind slate-200
          borderWidth: 0,
          hoverOffset: 0,
          weight: 0.1, // Make this less visually dominant
        },
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: 'bottom',
          labels: {
            color: textColor,
          },
        },
        title: {
          display: true,
          text: 'Tunnel Usage',
          color: textColor,
          font: { size: 16, weight: 'normal', },
        },
        tooltip: {
          filter(tooltipItem) {
            // Only show tooltips for the main dataset (index 0)
            return tooltipItem.datasetIndex === 0;
          }
        },
      }
    },
  };

  const chart = new Chart(
    document.getElementById(ident),
    chartConfig
  );

  return chart;
}

function initComputeChart(ident, textColor) {
  // Create the compute usage chart
  const chartConfig = {
    type: 'doughnut',
    data: {
      labels: ['Used', 'Available'],
      datasets: [
        {
          data: [ 0, 0 ],
          backgroundColor: [
            '#3b82f6', // Tailwind blue-500
            '#94a3b8' // Tailwind slate-400
          ],
          hoverOffset: 4,
        },
        // Background dataset (will show when main dataset is all zeros)
        {
          data: [ 1, 1 ],
          backgroundColor: ['#e2e8f0'], // Tailwind slate-200
          borderWidth: 0,
          hoverOffset: 0,
          weight: 0.1, // Make this less visually dominant
        },
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: 'bottom',
          labels: {
            color: textColor,
          },
        },
        title: {
          display: true,
          text: 'Compute Usage',
          color: textColor,
          font: { size: 16, weight: 'normal', },
        },
        tooltip: {
          filter(tooltipItem) {
            // Only show tooltips for the main dataset (index 0)
            return tooltipItem.datasetIndex === 0;
          }
        },
      }
    },
  };

  const chart = new Chart(
    document.getElementById(ident),
    chartConfig
  );

  return chart;
}

function initStorageChart(ident, textColor) {
  // Create the storage usage chart
  const chartConfig = {
    type: 'doughnut',
    data: {
      labels: ['Used', 'Available'],
      datasets: [
        {
          data: [ 0, 0 ],
          backgroundColor: [
            '#3b82f6', // Tailwind blue-500
            '#94a3b8' // Tailwind slate-400
          ],
          hoverOffset: 4,
        },
        // Background dataset (will show when main dataset is all zeros)
        {
          data: [ 1, 1 ],
          backgroundColor: ['#e2e8f0'], // Tailwind slate-200
          borderWidth: 0,
          hoverOffset: 0,
          weight: 0.1, // Make this less visually dominant
        },
      ]
    },
    options: {
      responsive: true,
      maintainAspectRatio: false,
      plugins: {
        legend: {
          position: 'bottom',
          labels: {
            color: textColor,
          },
        },
        title: {
          display: true,
          text: 'Storage Usage',
          color: textColor,
          font: { size: 16, weight: 'normal', },
        },
        tooltip: {
          filter(tooltipItem) {
            // Only show tooltips for the main dataset (index 0)
            return tooltipItem.datasetIndex === 0;
          }
        },
      }
    },
  };

  const chart = new Chart(
    document.getElementById(ident),
    chartConfig
  );

  return chart;
}

window.usageComponent = function(userId) {
  let spacesChart = null,
      tunnelsChart = null,
      computeChart = null,
      storageChart = null;

  return {
    loading: true,
    quota: {
      max_spaces: 0,
      compute_units: 0,
      storage_units: 0,
      number_spaces: 0,
      max_tunnels: 0,
      number_spaces_deployed: 0,
      used_compute_units: 0,
      used_storage_units: 0,
      used_tunnels: 0,
    },

    async init() {
      // Initialize the graphs
      const textColor = this.darkMode ? '#9ca3af' : '#6b7280'; // dark:text-gray-400 : text-gray-500

      spacesChart = initSpacesChart('spaceUsage', textColor);
      tunnelsChart = initTunnelChart('tunnelUsage', textColor);
      computeChart = initComputeChart('computeUsage', textColor);
      storageChart = initStorageChart('storageUsage', textColor);

      // Track the theme and adjust the label colors
      window.addEventListener('theme-change', (e) => {
        const col = e.detail.dark_theme ? '#9ca3af' : '#6b7280'; // dark:text-gray-400 : text-gray-500

        spacesChart.options.plugins.legend.labels.color = col;
        spacesChart.options.plugins.title.color = col;
        spacesChart.update();

        tunnelsChart.options.plugins.legend.labels.color = col;
        tunnelsChart.options.plugins.title.color = col;
        tunnelsChart.update();

        computeChart.options.plugins.legend.labels.color = col;
        computeChart.options.plugins.title.color = col;
        computeChart.update();

        storageChart.options.plugins.legend.labels.color = col;
        storageChart.options.plugins.title.color = col;
        storageChart.update();
      });

      await this.getUsage();

      // Start a timer to look for updates
      setInterval(async () => {
        await this.getUsage();
      }, 3000);
    },

    async getUsage() {
      const self = this;
      await fetch(`/api/users/${userId}/quota`, {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((quota) => {
            self.quota = quota;
            self.loading = false;

            if (spacesChart) {
              spacesChart.data.datasets[0].data = [
                self.quota.number_spaces_deployed,
                self.quota.number_spaces - self.quota.number_spaces_deployed,
                Math.max(0, self.quota.max_spaces ? self.quota.max_spaces - self.quota.number_spaces : 0)
              ];
              spacesChart.data.labels = [`Running ${self.quota.number_spaces_deployed}`, `Stopped ${self.quota.number_spaces - self.quota.number_spaces_deployed}`, `Available ${self.quota.max_spaces ? self.quota.max_spaces - self.quota.number_spaces : '-'}`];
              spacesChart.update();
            }

            if (tunnelsChart) {
              tunnelsChart.data.datasets[0].data = [
                self.quota.used_tunnels,
                Math.max(0, self.quota.max_tunnels ? self.quota.max_tunnels - self.quota.used_tunnels : 0)
              ];
              tunnelsChart.data.labels = [`Used ${self.quota.used_tunnels}`, `Available ${self.quota.max_tunnels ? self.quota.max_tunnels - self.quota.used_tunnels : '-'}`];
              tunnelsChart.update();
            }

            if (computeChart) {
              computeChart.data.datasets[0].data = [
                self.quota.used_compute_units,
                Math.max(0, self.quota.compute_units ? self.quota.compute_units - self.quota.used_compute_units : 0)
              ];
              computeChart.data.labels = [`Used ${self.quota.used_compute_units}`, `Available ${self.quota.compute_units ? self.quota.compute_units - self.quota.used_compute_units : '-'}`];
              computeChart.update();
            }

            if (storageChart) {
              storageChart.data.datasets[0].data = [
                self.quota.used_storage_units,
                Math.max(0, self.quota.storage_units ? self.quota.storage_units - self.quota.used_storage_units : 0)
              ];
              storageChart.data.labels = [`Used ${self.quota.used_storage_units}`, `Available ${self.quota.storage_units ? self.quota.storage_units - self.quota.used_storage_units : '-'}`];
              storageChart.update();
            }
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        window.location.href = '/logout';
      });
    },
  };
}
