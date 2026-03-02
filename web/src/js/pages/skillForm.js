import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-markdown";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";

import { focus } from "../focus.js";

window.skillForm = function (isEdit, skillId, isUserSkill = false) {
  return {
    loading: true,
    isEdit: isEdit,
    skillId: skillId,
    isUserSkill: isUserSkill,
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
    darkMode: Alpine.$persist(null).as("dark-theme").using(localStorage),

    async initData() {
      await this.loadGroups();

      if (this.isEdit) {
        await fetch(`/api/skill/${this.skillId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        })
          .then((response) => {
            if (response.status === 200) {
              response.json().then((data) => {
                this.formData = {
                  content: data.content,
                  groups: data.groups || [],
                  zones: data.zones || [],
                  active: data.active !== undefined ? data.active : true,
                  is_managed: data.is_managed || false,
                };
                this.isUserSkill = data.user_id ? true : false;
                this.zoneValid = [];
                this.formData.zones.forEach(() => {
                  this.zoneValid.push(true);
                });
                this.initEditor();
                this.loading = false;
              });
            } else if (response.status === 401) {
              window.location.href = "/logout";
            }
          })
          .catch(() => {});
      } else {
        // Default template for new skills
        this.formData.content = `---
name: "my-skill"
description: "Brief description of what this skill does"
---

# Skill Content

Add your skill documentation here in markdown format.
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
          this.contentEditor.setTheme(
            darkMode ? "ace/theme/github_dark" : "ace/theme/github",
          );
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
          if (e.detail.dark_theme) {
            this.contentEditor.setTheme("ace/theme/github_dark");
          } else {
            this.contentEditor.setTheme("ace/theme/github");
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

      this.contentValid = this.formData.content.length <= 4 * 1024 * 1024;

      if (!this.contentValid || !this.checkZonesValid()) {
        return;
      }

      const submitData = { ...this.formData };

      if (this.isUserSkill) {
        submitData.user_id = "current";
        submitData.groups = [];
      } else {
        submitData.user_id = "";
      }

      delete submitData.is_managed;

      if (!continueEditing) {
        this.loading = true;
      }

      const url = this.isEdit
        ? `/api/skill/${this.skillId}`
        : "/api/skill";
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
              msg: "Skill updated",
              type: "success",
            });
            if (!continueEditing) {
              this.$dispatch("close-skill-form");
            }
          } else if (response.status === 201) {
            const data = await response.json();
            this.$dispatch("show-alert", {
              msg: "Skill created",
              type: "success",
            });
            if (continueEditing) {
              this.skillId = data.skill_id;
              this.isEdit = true;
            } else {
              this.$dispatch("close-skill-form");
            }
          } else if (response.status === 401) {
            window.location.href = "/logout";
          } else {
            const text = await response.text();
            this.$dispatch("show-alert", {
              msg: text || "Failed to save skill",
              type: "error",
            });
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
