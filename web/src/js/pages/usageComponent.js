// Resource usage against quota, shown as one meter row per resource: the
// label, a bar filled to the share of the limit and the exact figures. A
// resource without a limit shows its figure and "No limit" instead of a bar.

const WARN_AT = 0.8;

function formatMB(mb) {
  if (mb >= 1024) return (mb / 1024).toFixed(1) + ' GB';
  if (mb > 0 && mb < 0.1) return '< 0.1 MB';
  return (Math.round(mb * 10) / 10) + ' MB';
}

function fillClass(state) {
  return state === 'full' ? 'bg-red-600' : (state === 'warn' ? 'bg-amber-500' : 'bg-blue-600');
}

// meter builds a row. used and limit are numbers; fmt renders a value.
function meter(key, label, used, limit, fmt = (v) => String(v)) {
  const row = { key, label, used, limit, segments: [], legend: [] };
  if (limit > 0) {
    const ratio = used / limit;
    row.percent = Math.min(100, Math.round(ratio * 100));
    row.state = ratio >= 1 ? 'full' : (ratio >= WARN_AT ? 'warn' : 'ok');
    row.text = `${fmt(used)} of ${fmt(limit)}`;
    row.valueText = `${fmt(used)} of ${fmt(limit)} used, ${row.percent}%` +
      (row.state === 'full' ? ', at limit' : (row.state === 'warn' ? ', near limit' : ''));
    row.segments = [{ percent: row.percent, cls: fillClass(row.state) }];
  } else {
    row.state = 'unlimited';
    row.text = `${fmt(used)} used`;
    row.valueText = `${fmt(used)} used, no limit`;
  }
  return row;
}

window.usageComponent = function(userId, filesEnabled = false) {
  return {
    loading: true,
    quota: null,

    get meters() {
      const q = this.quota;
      if (!q) return [];

      // Spaces split into running and stopped within the one bar.
      const total = q.number_spaces || 0;
      const running = q.number_spaces_deployed || 0;
      const stopped = total - running;
      const spaces = meter('spaces', 'Spaces', total, q.max_spaces || 0);
      if (spaces.state !== 'unlimited') {
        const runPct = Math.min(100, (running / q.max_spaces) * 100);
        const stopPct = Math.min(100 - runPct, (stopped / q.max_spaces) * 100);
        const light = spaces.state === 'full' ? 'bg-red-300 dark:bg-red-800'
          : (spaces.state === 'warn' ? 'bg-amber-300 dark:bg-amber-700' : 'bg-blue-300 dark:bg-blue-800');
        spaces.segments = [
          { percent: runPct, cls: fillClass(spaces.state) },
          { percent: stopPct, cls: light },
        ];
        spaces.legend = [
          { label: `Running ${running}`, cls: spaces.segments[0].cls },
          { label: `Stopped ${stopped}`, cls: light },
        ];
      }
      spaces.valueText += `, ${running} running, ${stopped} stopped`;
      if (spaces.state === 'unlimited' && total > 0) {
        spaces.text += ` (${running} running)`;
      }

      const rows = [
        spaces,
        meter('compute', 'Compute', q.used_compute_units || 0, q.compute_units || 0),
        meter('storage', 'Storage', q.used_storage_units || 0, q.storage_units || 0),
        meter('tunnels', 'Tunnels', q.used_tunnels || 0, q.max_tunnels || 0),
      ];
      if (filesEnabled) {
        rows.push(meter('files', 'File Storage', q.used_file_storage_mb || 0, q.file_storage_mb || 0, formatMB));
        rows.push(meter('buckets', 'Buckets', q.used_buckets || 0, q.max_buckets || 0));
      }
      return rows;
    },

    unsubscribers: [],

    destroy() {
      this.unsubscribers.forEach((off) => off());
      this.unsubscribers = [];
    },

    async init() {
      await this.getUsage();

      // Subscribe to SSE for real-time updates when spaces change
      if (window.sseClient) {
        this.unsubscribers = ['space:changed', 'space:deleted'].map((event) =>
          window.sseClient.subscribe(event, () => this.getUsage()));
      }
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
          });
        } else if (response.status === 401) {
          window.location.href = '/logout';
        }
      }).catch(() => {
        // Don't logout on network errors - Safari closes connections aggressively
      });
    },
  };
}
