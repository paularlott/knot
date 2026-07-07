import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-markdown";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/ext-searchbox";

import { focus } from "../focus.js";

window.commandForm = function (isEdit, commandId, isUserCommand) {
  return {
    loading: true,
    isEdit: isEdit,
    commandId: commandId,
    isUserCommand: isUserCommand,
    contentValid: true,
    formData: {
      content: "",
      groups: [],
      zones: [],
      active: true,
      is_managed: false,
    },
    availableGroups: [],
    zoneValid: [],
    contentEditor: null,

    async initData() {
      await this.loadGroups();

      if (this.isEdit) {
        await fetch(`/api/command/${this.commandId}`, {
          headers: { "Content-Type": "application/json" },
        })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                const fm = [
                  "---",
                  `name: "${data.name}"`,
                  `description: "${data.description}"`,
                ];
                if (data.argument_hint) fm.push(`argument-hint: "${data.argument_hint}"`);
                if (data.allowed_tools && data.allowed_tools.length > 0) fm.push(`allowed-tools: "${data.allowed_tools.join(", ")}"`);
                fm.push("---", "");

                this.formData = {
                  content: fm.join("\n") + (data.body || ""),
                  groups: data.groups || [],
                  zones: data.zones || [],
                  active: data.active !== undefined ? data.active : true,
                  is_managed: data.is_managed || false,
                };
                this.isUserCommand = data.user_id ? true : false;
                this.zoneValid = [];
                this.formData.zones.forEach(() => this.zoneValid.push(true));
                this.initEditor();
                this.loading = false;
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
          })
          .catch(() => {});
      } else {
        this.formData.content = `---
name: "my-command"
description: "Brief description shown in the command picker"
argument-hint: "<optional-argument-hint>"
allowed-tools: ""
---

Command body (markdown). Use \`$ARGUMENTS\` to insert the user's argument.
`;
        this.initEditor();
        this.loading = false;
      }
    },

    initEditor() {
      this.$nextTick(() => {
        let darkMode = JSON.parse(localStorage.getItem("_x_darkMode"));
        if (darkMode == null) darkMode = true;

        if (!this.contentEditor) {
          this.contentEditor = ace.edit("content");
          this.contentEditor.session.setValue(this.formData.content);
          this.contentEditor.session.on("change", () => {
            this.formData.content = this.contentEditor.getValue();
          });
          this.contentEditor.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
          this.contentEditor.session.setMode("ace/mode/markdown");
          if (this.formData.is_managed) {
            this.contentEditor.setReadOnly(true);
            this.contentEditor.renderer.$cursorLayer.element.style.display = "none";
          }
          this.contentEditor.setOptions({
            printMargin: false,
            newLineMode: "unix",
            tabSize: 2,
            wrap: true,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
          });
        }

        window.addEventListener("theme-change", (e) => {
          if (this.contentEditor) {
            this.contentEditor.setTheme(e.detail.dark_theme ? "ace/theme/github_dark" : "ace/theme/github");
          }
        });
      });
    },

    async loadGroups() {
      await fetch("/api/groups", { headers: { "Content-Type": "application/json" } })
        .then((response) => {
          if (response.status === 200) {
            response.json().then((data) => { this.availableGroups = data.groups || []; });
          }
        })
        .catch(() => {});
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
        let isValid = this.formData.zones[index].length > 0 && this.formData.zones[index].length <= 64;
        if (isValid) {
          for (let i = 0; i < this.formData.zones.length; i++) {
            if (i !== index && this.formData.zones[i] === this.formData.zones[index]) {
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
      let valid = true;
      this.formData.zones.forEach((zone, index) => { valid = valid && this.zoneValid[index]; });
      return valid;
    },

    toggleGroup(groupId) {
      const index = this.formData.groups.indexOf(groupId);
      if (index >= 0) this.formData.groups.splice(index, 1);
      else this.formData.groups.push(groupId);
    },

    async submitData(continueEditing = false) {
      if (this.formData.is_managed) return;
      this.contentValid = this.formData.content.length <= 1 * 1024 * 1024;
      if (!this.contentValid || !this.checkZonesValid()) return;

      const submitData = { ...this.formData };
      if (this.isUserCommand) {
        submitData.user_id = "current";
        submitData.groups = [];
      } else {
        submitData.user_id = "";
      }
      delete submitData.is_managed;

      if (!continueEditing) this.loading = true;

      const url = this.isEdit ? `/api/command/${this.commandId}` : "/api/command";
      const method = this.isEdit ? "PUT" : "POST";

      await fetch(url, {
        method: method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(submitData),
      })
        .then(async (response) => {
          if (response.status === 200 || response.status === 201) {
            this.$dispatch("show-alert", {
              msg: this.isEdit ? "Command updated" : "Command created",
              type: "success",
            });
            if (continueEditing) {
              if (response.status === 201) {
                const data = await response.json();
                if (data.command_id) {
                  this.isEdit = true;
                  this.commandId = data.command_id;
                }
              }
            } else {
              this.$dispatch("close-command-form");
            }
          } else if (response.status === 400) {
            const data = await response.json();
            this.$dispatch("show-alert", { msg: data.error || "Validation error", type: "error" });
            this.loading = false;
          } else if (response.status === 403) {
            const data = await response.json();
            this.$dispatch("show-alert", { msg: data.error || "Permission denied", type: "error" });
            this.loading = false;
          } else if (response.status === 401) {
            window.location.href = "/logout";
          }
        })
        .catch(() => {
          this.$dispatch("show-alert", { msg: "Network error", type: "error" });
          this.loading = false;
        });
    },

    get formValid() {
      return this.contentValid && this.checkZonesValid();
    },
  };
};
