/// wysiwyg editor
import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-python";
import "ace-builds/src-noconflict/mode-toml";
import "ace-builds/src-noconflict/mode-text";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";
import "ace-builds/src-noconflict/ext-language_tools";
import "ace-builds/src-noconflict/snippets/python";
import "ace-builds/src-noconflict/snippets/text";

import { focus } from "../focus.js";
import "./aceEditorCompleter.js"; // Sets up window.AceEditorCompleter
import { scriptLibraries } from "./scriptCompletions.js";

window.scriptForm = function (isEdit, scriptId, isUserScript = false) {
  return {
    loading: true,
    isEdit: isEdit,
    scriptId: scriptId,
    isUserScript: isUserScript,
    nameValid: true,
    contentValid: true,
    formData: {
      name: "",
      description: "",
      content: "",
      groups: [],
      zones: [],
      active: true,
      script_type: "script",
      mcp_input_schema_toml: "",
      mcp_keywords: [],
      on_demand_tool: false,
      is_managed: false,
    },
    mcpKeywordsStr: "",
    availableGroups: [],
    zoneValid: [],
    contentEditor: null,
    schemaEditor: null,
    descriptionEditor: null,
    darkMode: Alpine.$persist(null).as("dark-theme").using(localStorage),

    async initData() {
      focus.Element('input[name="name"]');

      await this.loadGroups();

      if (this.isEdit) {
        await fetch(`/api/scripts/${this.scriptId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                this.formData = {
                  name: data.name,
                  description: data.description,
                  content: data.content,
                  groups: data.groups || [],
                  zones: data.zones || [],
                  active: data.active,
                  script_type: data.script_type || "script",
                  mcp_input_schema_toml: data.mcp_input_schema_toml || "",
                  mcp_keywords: data.mcp_keywords || [],
                  on_demand_tool: data.on_demand_tool || false,
                  is_managed: data.is_managed || false,
                };
                this.isUserScript = data.user_id ? true : false;
                this.mcpKeywordsStr = this.formData.mcp_keywords.join(", ");
                this.zoneValid = [];
                this.formData.zones.forEach(() => {
                  this.zoneValid.push(true);
                });
                this.initEditors();
                this.loading = false;
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
          })
          .catch(() => {});
      } else {
        this.initEditors();
        this.loading = false;
      }
    },

    initEditors() {
      this.$nextTick(() => {
        let darkMode = JSON.parse(localStorage.getItem("_x_darkMode"));
        if (darkMode == null) darkMode = true;

        if (!this.contentEditor) {
          this.contentEditor = ace.edit("content");
          this.contentEditor.session.setValue(this.formData.content);
          this.contentEditor.session.on("change", () => {
            this.formData.content = this.contentEditor.getValue();
          });
          this.contentEditor.setTheme(
            darkMode ? "ace/theme/github_dark" : "ace/theme/github",
          );
          this.contentEditor.session.setMode("ace/mode/python");
          this.contentEditor.setReadOnly(this.formData.is_managed);

          // Register custom completer for scriptling/knot libraries
          window.AceEditorCompleter.setup(this.contentEditor, scriptLibraries, {
            debug: false,
          });

          this.contentEditor.setOptions({
            printMargin: false,
            newLineMode: "unix",
            tabSize: 2,
            wrap: false,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
            enableBasicAutocompletion: true,
            enableLiveAutocompletion: true,
            enableSnippets: true,
          });
        }

        if (!this.schemaEditor) {
          this.schemaEditor = ace.edit("mcp_schema");
          this.schemaEditor.session.setValue(
            this.formData.mcp_input_schema_toml,
          );
          this.schemaEditor.session.on("change", () => {
            this.formData.mcp_input_schema_toml = this.schemaEditor.getValue();
          });
          this.schemaEditor.setTheme(
            darkMode ? "ace/theme/github_dark" : "ace/theme/github",
          );
          this.schemaEditor.session.setMode("ace/mode/toml");
          this.schemaEditor.setReadOnly(this.formData.is_managed);
          this.schemaEditor.setOptions({
            printMargin: false,
            newLineMode: "unix",
            tabSize: 2,
            wrap: false,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
            useWorker: false,
            enableBasicAutocompletion: true,
            enableLiveAutocompletion: true,
            enableSnippets: true,
          });
        }

        if (!this.descriptionEditor) {
          this.descriptionEditor = ace.edit("description");
          this.descriptionEditor.session.setValue(this.formData.description);
          this.descriptionEditor.session.on("change", () => {
            this.formData.description = this.descriptionEditor.getValue();
          });
          this.descriptionEditor.setTheme(
            darkMode ? "ace/theme/github_dark" : "ace/theme/github",
          );
          this.descriptionEditor.session.setMode("ace/mode/text");
          this.descriptionEditor.setReadOnly(this.formData.is_managed);
          this.descriptionEditor.setOptions({
            printMargin: false,
            newLineMode: "unix",
            tabSize: 2,
            wrap: false,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
            useWorker: false,
          });
        }

        window.addEventListener("theme-change", (e) => {
          if (e.detail.dark_theme) {
            this.contentEditor.setTheme("ace/theme/github_dark");
            this.schemaEditor.setTheme("ace/theme/github_dark");
            this.descriptionEditor.setTheme("ace/theme/github_dark");
          } else {
            this.contentEditor.setTheme("ace/theme/github");
            this.schemaEditor.setTheme("ace/theme/github");
            this.descriptionEditor.setTheme("ace/theme/github");
          }
        });
      });
    },

    async loadGroups() {
      await fetch("/api/groups", {
        headers: {
          "Content-Type": "application/json",
        },
      })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              this.availableGroups = data.groups || [];
            });
          }
        })
        .catch(() => {});
    },

    checkName() {
      this.nameValid = /^[a-zA-Z0-9_]{1,64}$/.test(this.formData.name);
    },

    addZone() {
      this.zoneValid.push(true);
      this.formData.zones.push("");
    },

    removeZone(index) {
      this.formData.zones.splice(index, 1);
      this.zoneValid.splice(index, 1);
    },

    checkZone(index) {
      if (index >= 0 && index < this.formData.zones.length) {
        let isValid =
          this.formData.zones[index].length > 0 &&
          this.formData.zones[index].length <= 64;
        if (isValid) {
          for (let i = 0; i < this.formData.zones.length; i++) {
            if (
              i !== index &&
              this.formData.zones[i] === this.formData.zones[index]
            ) {
              isValid = false;
              break;
            }
          }
        }
        this.zoneValid[index] = isValid;
        return isValid;
      }
      return false;
    },

    checkZonesValid() {
      let zonesValid = true;
      this.formData.zones.forEach((zone, index) => {
        zonesValid = zonesValid && this.zoneValid[index];
      });
      return zonesValid;
    },

    toggleGroup(groupId) {
      const index = this.formData.groups.indexOf(groupId);
      if (index >= 0) {
        this.formData.groups.splice(index, 1);
      } else {
        this.formData.groups.push(groupId);
      }
    },

    async submitData(continueEditing = false) {
      if (this.formData.is_managed) return;

      this.checkName();
      this.contentValid = this.formData.content.length <= 4 * 1024 * 1024;

      if (!this.nameValid || !this.contentValid || !this.checkZonesValid()) {
        return;
      }

      this.formData.mcp_keywords = this.mcpKeywordsStr
        .split(",")
        .map((k) => k.trim())
        .filter((k) => k.length > 0);

      const submitData = { ...this.formData };

      // Set user_id for user scripts
      if (this.isUserScript) {
        submitData.user_id = "current"; // Backend will set to current user
        submitData.groups = []; // User scripts don't have groups
      } else {
        submitData.user_id = ""; // Global script
      }

      delete submitData.is_managed; // Don't send this field

      if (!continueEditing) {
        this.loading = true;
      }

      const url = this.isEdit
        ? `/api/scripts/${this.scriptId}`
        : "/api/scripts";
      const method = this.isEdit ? "PUT" : "POST";

      await fetch(url, {
        method: method,
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(submitData),
      })
        .then(async (response) => {
          if (response.status === 200) {
            this.$dispatch("show-alert", {
              msg: "Script updated",
              type: "success",
            });
            if (!continueEditing) {
              this.$dispatch("close-script-form");
            }
          } else if (response.status === 201) {
            const data = await response.json();
            this.$dispatch("show-alert", {
              msg: "Script created",
              type: "success",
            });
            if (continueEditing) {
              this.scriptId = data.script_id;
              this.isEdit = true;
            } else {
              this.$dispatch("close-script-form");
            }
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            try {
              const data = await response.json();
              this.$dispatch("show-alert", {
                msg: data.error || "Failed to save script",
                type: "error",
              });
            } catch (e) {
              const text = await response.text();
              this.$dispatch("show-alert", {
                msg: text || "Failed to save script",
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
