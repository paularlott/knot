import { focus } from "../focus.js";

window.mcpServerForm = function (isEdit, serverId) {
  return {
    loading: true,
    isEdit: isEdit,
    serverId: serverId,
    transportType: "http",
    namespaceValid: true,
    urlValid: true,
    commandValid: true,
    argsText: "",
    envText: "",
    formData: {
      namespace: "",
      url: "",
      command: "",
      args: [],
      env: [],
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
                  env: data.env || [],
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
                this.envText = (this.formData.env || []).join("\n");
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

    checkNamespace() {
      this.namespaceValid = (this.formData.namespace || "").trim().length > 0;
      return this.namespaceValid;
    },
    checkUrl() {
      this.urlValid = /^https?:\/\/\S+$/i.test((this.formData.url || "").trim());
      return this.urlValid;
    },
    checkCommand() {
      this.commandValid = (this.formData.command || "").trim().length > 0;
      return this.commandValid;
    },

    async submitData(continueEditing = false) {
      let err = !this.checkNamespace();
      if (this.transportType === "stdio") {
        this.urlValid = true;
        err = !this.checkCommand() || err;
      } else {
        this.commandValid = true;
        err = !this.checkUrl() || err;
      }
      if (err) {
        window.knotToast("Some fields need attention.", "error");
        focus.firstInvalid(this.$root);
        return;
      }

      const submitData = { ...this.formData };
      delete submitData.user_id;
      if (this.transportType === "stdio") {
        submitData.url = "";
        submitData.args = this.parseArgs(this.argsText);
        submitData.env = (this.envText || "")
          .split(/\n/)
          .map((l) => l.trim())
          .filter(Boolean);
      } else {
        submitData.command = "";
        submitData.args = [];
        submitData.env = [];
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
            window.knotError(this.isEdit ? "save the MCP server" : "create the MCP server", response);
          }
          this.loading = false;
        })
        .catch((err) => {
          window.knotError(this.isEdit ? "save the MCP server" : "create the MCP server", err);
          this.loading = false;
        });
    },
  };
};
