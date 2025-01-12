window.usageComponent = function(userId) {
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
      this.getUsage();

      // Start a timer to look for updates
      setInterval(async () => {
        this.getUsage();
      }, 3000);
    },

    async getUsage() {
      let self = this;
      const quotaResponse = await fetch('/api/v1/users/' + userId + '/quota', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      self.quota = await quotaResponse.json();
      self.loading = false;
    },
  };
}
