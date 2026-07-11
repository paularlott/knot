window.mcpServerForm = function (isEdit, serverId) {
  return {
    loading: true,
    isEdit: isEdit,
    serverId: serverId,
    transportType: "http",
    argsText: "",
    formData: {
      namespace: "",
      url: "",
      command: "",
      args: [],
      auth_type: "",
      token: "",
      oauth_client_id: "",
      oauth_token_url: "",
      oauth_access_token: "",
      oauth_refresh_token: "",
      enabled: true,
      tool_visibility: "native",
      disabled_tools: [],
      remote_search: false,
    },

    async initData() {
      if (this.isEdit && this.serverId) {
        await fetch(`/api/mcp-servers/${this.serverId}`, {
          headers: { "Content-Type": "application/json" },
        })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                this.formData = {
                  namespace: data.namespace || "",
                  url: data.url || "",
                  command: data.command || "",
                  args: data.args || [],
                  auth_type: data.auth_type || "",
                  token: data.token || "",
                  oauth_client_id: data.oauth_client_id || "",
                  oauth_token_url: data.oauth_token_url || "",
                  oauth_access_token: data.oauth_access_token || "",
                  oauth_refresh_token: data.oauth_refresh_token || "",
                  enabled: data.enabled !== undefined ? data.enabled : true,
                  tool_visibility: data.tool_visibility || "native",
                  disabled_tools: data.disabled_tools || [],
                  remote_search: data.remote_search || false,
                };
                this.transportType = this.formData.command ? "stdio" : "http";
                this.argsText = (this.formData.args || []).join(" ");
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
      } else {
        this.loading = false;
      }
    },

    parseArgs(text) {
      const args = [];
      const regex = /"([^"]*)"|'([^']*)'|(\S+)/g;
      let match;
      while ((match = regex.exec(text)) !== null) {
        args.push(match[1] || match[2] || match[3]);
      }
      return args;
    },

    async submitData(continueEditing = false) {
      if (!this.formData.namespace) {
        this.$dispatch("show-alert", {
          msg: "Namespace is required",
          type: "error",
        });
        return;
      }

      const submitData = { ...this.formData };
      delete submitData.user_id;
      if (this.transportType === "stdio") {
        submitData.url = "";
        submitData.args = this.parseArgs(this.argsText);
      } else {
        submitData.command = "";
        submitData.args = [];
        if (!submitData.url) {
          this.$dispatch("show-alert", {
            msg: "URL is required for HTTP transport",
            type: "error",
          });
          return;
        }
      }

      if (!continueEditing) {
        this.loading = true;
      }

      const url = this.isEdit
        ? `/api/mcp-servers/${this.serverId}`
        : "/api/mcp-servers";
      const method = this.isEdit ? "PUT" : "POST";

      await fetch(url, {
        method: method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(submitData),
      })
        .then(async (response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "MCP server updated",
              type: "success",
            });
            if (!continueEditing) {
              this.$dispatch("close-mcp-server-form");
            }
          } else if (response.status === 201) {
            const data = await response.json();
            this.$dispatch("show-alert", {
              msg: "MCP server created",
              type: "success",
            });
            if (continueEditing) {
              this.serverId = data.mcp_server_id;
              this.isEdit = true;
            } else {
              this.$dispatch("close-mcp-server-form");
            }
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            const text = await response.text();
            try {
              const errData = JSON.parse(text);
              this.$dispatch("show-alert", {
                msg: errData.error || "Failed to save MCP server",
                type: "error",
              });
            } catch {
              this.$dispatch("show-alert", {
                msg: "Failed to save MCP server",
                type: "error",
              });
            }
          }
          this.loading = false;
        })
        .catch((err) => {
          this.$dispatch("show-alert", {
            msg: "Network error: " + err.message,
            type: "error",
          });
          this.loading = false;
        });
    },
  };
};
