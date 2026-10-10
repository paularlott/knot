window.pluginsListComponent = function () {
  return {
    loading: true,
    plugins: [],
    failed: [],
    warnings: [],

    async init() {
      let response;
      try {
        response = await fetch("/api/plugins", {
          headers: {
            "Content-Type": "application/json",
          },
        });
      } catch (err) {
        this.loading = false;
        window.knotError("load the plugins", err);
        return;
      }

      if (response.status === 401) {
        window.location.href = "/logout";
        return;
      }

      if (response.status === 200) {
        const list = await response.json();
        this.plugins = list.plugins || [];
        this.failed = list.failed || [];
        this.warnings = list.warnings || [];
      } else {
        window.knotError("load the plugins", response);
      }

      this.loading = false;
    },
  };
};
