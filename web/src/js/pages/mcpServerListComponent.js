import Alpine from "alpinejs";

window.mcpServerListComponent = function (userId, isLeafNode) {
  document.addEventListener("keydown", (e) => {
    if ((e.metaKey || e.ctrlKey) && e.key === "k") {
      e.preventDefault();
      document.getElementById("search").focus();
    }
  });

  return {
    loading: true,
    servers: [],
    searchTerm: "",
    currentUserId: userId || "",
    isLeafNode: isLeafNode || false,
    deleteConfirm: {
      show: false,
      server: { mcp_server_id: "", namespace: "" },
    },
    serverFormModal: {
      show: false,
      isEdit: false,
      serverId: "",
    },
    toolsModal: {
      show: false,
      loading: false,
      error: "",
      serverId: "",
      namespace: "",
      tools: [],
    },

    async init() {
      await this.getServers();

      if (window.sseClient) {
        window.sseClient.subscribe("mcp-servers:changed", (payload) => {
          if (payload?.id) this.getServers();
        });

        window.sseClient.subscribe("mcp-servers:deleted", (payload) => {
          this.servers = this.servers.filter(
            (x) => x.mcp_server_id !== payload?.id,
          );
          this.applyFilters();
        });

        window.sseClient.subscribe("reconnected", () => {
          this.getServers();
        });
      }
    },

    async getServers() {
      await fetch("/api/mcp-servers", {
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              this.servers = (data.servers || []).slice();
              this.servers.sort((a, b) =>
                a.namespace.localeCompare(b.namespace),
              );
              this.applyFilters();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.loading = false;
          }
        })
        .catch(() => {
          this.loading = false;
        });
    },

    createServer() {
      this.serverFormModal.isEdit = false;
      this.serverFormModal.serverId = "";
      this.serverFormModal.show = true;
    },

    editServer(serverId) {
      this.serverFormModal.isEdit = true;
      this.serverFormModal.serverId = serverId;
      this.serverFormModal.show = true;
    },

    canEdit(server) {
      return true;
    },

    canDelete(server) {
      if (this.isLeafNode) return false;
      return this.isLeafNode ? false : true;
    },

    async openTools(server) {
      this.toolsModal.serverId = server.mcp_server_id;
      this.toolsModal.namespace = server.namespace;
      this.toolsModal.tools = [];
      this.toolsModal.error = "";
      this.toolsModal.loading = true;
      this.toolsModal.show = true;

      try {
        const resp = await fetch(`/api/mcp-servers/${server.mcp_server_id}/tools`);
        if (resp.ok) {
          const data = await resp.json();
          this.toolsModal.tools = data.tools || [];
          if (data.error) {
            this.toolsModal.error = data.error;
          }
        } else {
          this.toolsModal.error = "Failed to load tools.";
        }
      } catch {
        this.toolsModal.error = "Failed to connect to the remote server.";
      }
      this.toolsModal.loading = false;
    },

    async toggleToolModal(tool, enabled) {
      const prev = tool.enabled;
      tool.enabled = enabled;
      try {
        const resp = await fetch(`/api/mcp-servers/${this.toolsModal.serverId}/toggle-tool`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ tool_name: tool.name, enabled: tool.enabled }),
        });
        if (!resp.ok) {
          tool.enabled = prev;
        }
      } catch {
        tool.enabled = prev;
      }
    },

    async toggleAllTools(allEnabled) {
      const target = !allEnabled;
      for (const tool of this.toolsModal.tools) {
        if (tool.enabled !== target) {
          await this.toggleToolModal(tool, target);
        }
      }
    },

    async deleteServer(serverId) {
      await fetch(`/api/mcp-servers/${serverId}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
      })
        .then((response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "MCP server deleted",
              type: "success",
            });
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            this.$dispatch("show-alert", {
              msg: "MCP server could not be deleted",
              type: "error",
            });
          }
        })
        .catch(() => {});
      this.getServers();
    },

    searchChanged() {
      this.applyFilters();
    },

    applyFilters() {
      const term = this.searchTerm.toLowerCase();
      this.servers.forEach((s) => {
        let show = true;
        if (term.length > 0) {
          const inNs = (s.namespace || "").toLowerCase().includes(term);
          const inUrl = (s.url || "").toLowerCase().includes(term);
          const inCmd = (s.command || "").toLowerCase().includes(term);
          show = show && (inNs || inUrl || inCmd);
        }
        s.searchHide = !show;
      });
    },
  };
};
