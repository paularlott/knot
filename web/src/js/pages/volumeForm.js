import { validate } from "../validators.js";
import { focus } from "../focus.js";

/// wysiwyg editor
import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-terraform";
import "ace-builds/src-noconflict/mode-yaml";
import "ace-builds/src-noconflict/mode-text";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";
import "ace-builds/src-noconflict/ext-language_tools";
import { setSpecCompleter } from "./aceSpecCompleter.js";
import {
  localVolumeSpecCompletions,
  nomadVolumeSpecCompletions,
} from "./specCompletions.js";

window.volumeForm = function (isEdit, volumeId) {
  return {
    formData: {
      name: "",
      definition: "",
      platform: "nomad",
      node_id: "",
      active: false,
    },
    loading: true,
    nameValid: true,
    volValid: true,
    isEdit,
    showPlatformWarning: false,
    availableNodes: [],
    loadingNodes: false,
    specErrors: [],
    volumeEditor: null,

    async initData() {
      focus.Element('input[name="name"]');

      if (isEdit) {
        const volumeResponse = await fetch(`/api/volumes/${volumeId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        });

        if (volumeResponse.status !== 200) {
          window.location.href = "/volumes";
        } else {
          const volume = await volumeResponse.json();

          this.formData.name = volume.name;
          this.formData.definition = volume.definition;
          this.formData.platform = volume.platform;
          this.formData.node_id = volume.node_id || "";
          this.formData.active = volume.active || false;
        }
      }

      await this.fetchNodes();

      let darkMode = JSON.parse(localStorage.getItem("_x_darkMode"));
      if (darkMode == null) darkMode = true;

      // Create the volume editor
      const editorVol = ace.edit("vol");
      this.volumeEditor = editorVol;
      editorVol.session.setValue(this.formData.definition);
      editorVol.session.on("change", () => {
        this.formData.definition = editorVol.getValue();
        this.specErrors = [];
        this.volValid = true;
        editorVol.session.clearAnnotations();
      });
      editorVol.setTheme(
        darkMode ? "ace/theme/github_dark" : "ace/theme/github",
      );
      editorVol.session.setMode("ace/mode/yaml");
      editorVol.setOptions({
        printMargin: false,
        newLineMode: "unix",
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      this.$watch("formData.platform", () => {
        this.applyVolumeCompleter();
        this.specErrors = [];
        this.volValid = true;
        this.volumeEditor.session.clearAnnotations();
      });

      this.applyVolumeCompleter();

      // Listen for the theme_change event on the body & change the editor theme
      window.addEventListener("theme-change", (e) => {
        if (e.detail.dark_theme) {
          editorVol.setTheme("ace/theme/github_dark");
        } else {
          editorVol.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    async fetchNodes(clearIfNotFound = false) {
      const platform = this.formData.platform;
      if (platform === 'nomad') {
        this.availableNodes = [];
        return;
      }
      this.loadingNodes = true;
      try {
        const response = await fetch(`/api/volumes/nodes?platform=${platform}`, {
          headers: { "Content-Type": "application/json" },
        });
        if (response.status === 200) {
          const nodes = await response.json();
          this.availableNodes = nodes || [];
          // Auto-select if only one node and no node already chosen
          if (this.availableNodes.length === 1 && !this.formData.node_id) {
            this.formData.node_id = this.availableNodes[0].node_id;
          }
          // Only clear node_id if explicitly requested (i.e. platform changed)
          if (clearIfNotFound && this.formData.node_id && !this.availableNodes.find(n => n.node_id === this.formData.node_id)) {
            this.formData.node_id = this.availableNodes.length === 1 ? this.availableNodes[0].node_id : "";
          }
        } else {
          this.availableNodes = [];
        }
      } catch {
        this.availableNodes = [];
      }
      this.loadingNodes = false;
    },
    async onPlatformChange() {
      this.formData.node_id = "";
      await this.fetchNodes(true);
    },
    checkName() {
      this.nameValid = validate.name(this.formData.name);
      return this.nameValid;
    },
    checkVol() {
      this.volValid = validate.required(this.formData.definition);
      return this.volValid;
    },
    applyVolumeCompleter() {
      if (!this.volumeEditor) {
        return;
      }

      setSpecCompleter(
        this.volumeEditor,
        this.formData.platform === "nomad"
          ? nomadVolumeSpecCompletions
          : localVolumeSpecCompletions,
      );
    },
    setEditorErrors(messages) {
      if (!this.volumeEditor) {
        return;
      }

      this.volumeEditor.session.setAnnotations(
        messages.map((message, index) => ({
          row: index,
          column: 0,
          text: message,
          type: "error",
        })),
      );
    },
    async validateSpec() {
      const response = await fetch("/api/volumes/validate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          platform: this.formData.platform,
          definition: this.formData.definition,
        }),
      });

      const result = await response.json();
      this.specErrors = (result.errors || []).map((error) => error.message);
      this.volValid = this.specErrors.length === 0;
      this.setEditorErrors(this.specErrors);

      return response.ok && !!result.valid;
    },
    checkPlatform() {
      return validate.isOneOf(this.formData.platform, [
        "docker",
        "podman",
        "nomad",
        "apple",
        "container",
      ]);
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkVol() || err;
      err = !this.checkPlatform() || err;
      if (err) {
        this.$dispatch("show-alert", {
          msg: "Please fix the validation errors before saving",
          type: "error",
        });
        return;
      }

      try {
        const specValid = await this.validateSpec();
        if (!specValid) {
          this.$dispatch("show-alert", {
            msg: "Please fix the volume definition errors before saving",
            type: "error",
          });
          return;
        }
      } catch (error) {
        self.$dispatch("show-alert", {
          msg: `Failed to validate the volume, ${error.message}`,
          type: "error",
        });
        return;
      }

      this.loading = true;

      const data = {
        name: this.formData.name,
        definition: this.formData.definition,
        platform: this.formData.platform,
        node_id: this.formData.node_id,
      };

      await fetch(isEdit ? `/api/volumes/${volumeId}` : "/api/volumes", {
        method: isEdit ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(data),
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Volume updated",
              type: "success",
            });
            self.$dispatch("close-volume-form");
          } else if (response.status === 201) {
            self.$dispatch("show-alert", {
              msg: "Volume created",
              type: "success",
            });
            self.$dispatch("close-volume-form");
          } else {
            response.json().then((d) => {
              self.$dispatch("show-alert", {
                msg: `Failed to update the volume, ${d.error}`,
                type: "error",
              });
            });
          }
        })
        .catch((error) => {
          self.$dispatch("show-alert", {
            msg: `Error!<br />${error.message}`,
            type: "error",
          });
        })
        .finally(() => {
          this.loading = false;
        });
    },
  };
};
