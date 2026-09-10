window.pluginsListComponent = function () {
  return {
    loading: true,
    plugins: [],
    failed: [],
    warnings: [],

    async init() {
      const response = await fetch("/api/plugins", {
        headers: {
          "Content-Type": "application/json",
        },
      });

      if (response.status === 401) {
        window.location.href = "/logout";
        return;
      }

      if (response.status === 200) {
        const list = await response.json();
        this.plugins = list.plugins || [];
        this.failed = list.failed || [];
        this.warnings = list.warnings || [];
      }

      this.loading = false;
    },
  };
};
