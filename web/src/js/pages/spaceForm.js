import { validate } from "../validators.js";
import { focus } from "../focus.js";
import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-text";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";

window.spaceForm = function (
  isEdit,
  spaceId,
  userId,
  preferredShell,
  forUserId,
  forUserUsername,
  templateId,
  canSetSpaceDependencies,
) {
  return {
    iconList: [],
    scriptList: [],
    dependencyCatalog: [],
    dependencyOptions: [],
    dependencyModal: {
      show: false,
      selectedId: "",
      selectedName: "",
      invalid: false,
    },
    dependencyRemoveConfirm: {
      show: false,
      id: "",
      name: "",
    },
    dependencyTargetZone: "",
    canSetSpaceDependencies,
    formData: {
      name: "",
      description: "",
      icon_url: "",
      template_id: templateId,
      shell: preferredShell,
      user_id: forUserId,
      alt_names: [],
      custom_fields: [],
      created_at: "",
      created_at_formatted: "",
      selected_node_id: "",
      startup_script_id: "",
      depends_on: [],
    },
    template_id: templateId,
    template: {
      custom_fields: [],
    },
    isManual: false,
    loading: true,
    buttonLabelWorking: isEdit ? "Saving..." : "Creating...",
    nameValid: true,
    addressValid: true,
    forUsername: forUserUsername,
    volume_size_valid: {},
    volume_size_label: {},
    isEdit,
    stayOnPage: true,
    altNameValid: [],
    descValid: true,
    startOnCreate: true,
    saving: false,
    quotaStorageLimitShow: false,
    availableNodes: [],
    loadingNodes: false,

    async loadDependencyOptions(ownerId) {
      if (!this.canSetSpaceDependencies) {
        this.dependencyCatalog = [];
        this.dependencyOptions = [];
        return;
      }

      if (!ownerId) {
        this.dependencyCatalog = [];
        this.dependencyOptions = [];
        return;
      }

      const response = await fetch(
        `/api/spaces?user_id=${encodeURIComponent(ownerId)}`,
        {
          headers: {
            "Content-Type": "application/json",
          },
        },
      );

      if (response.status !== 200) {
        this.dependencyCatalog = [];
        this.dependencyOptions = [];
        return;
      }

      const data = await response.json();
      this.dependencyCatalog = (data.spaces || [])
        .filter((space) => space.user_id === ownerId && space.space_id !== spaceId)
        .map((space) => ({
          id: space.space_id,
          name: space.name,
          description: space.description || "",
          is_deployed: space.is_deployed,
          zone: space.zone || "",
          is_remote: space.is_remote,
        }));
      this.refreshDependencyOptions();
    },

    dependencyDescription(option) {
      return option.description
        ? `${option.description} (${option.is_deployed ? "running" : "stopped"})`
        : option.is_deployed
          ? "Running"
          : "Stopped";
    },

    isDependencyInZone(option) {
      if (this.isEdit && this.dependencyTargetZone !== "") {
        return option.zone === this.dependencyTargetZone;
      }
      return !option.is_remote;
    },

    refreshDependencyOptions() {
      this.dependencyOptions = this.dependencyCatalog.filter(
        (option) =>
          this.isDependencyInZone(option) &&
          !this.formData.depends_on.includes(option.id),
      );
      this.$nextTick(() => {
        this.$dispatch("refresh-space-autocompleter");
      });
    },

    dependencyById(id) {
      return this.dependencyCatalog.find((option) => option.id === id) || null;
    },

    selectedDependencies() {
      return this.formData.depends_on.map((id) => {
        const option = this.dependencyById(id);
        return option || {
          id,
          name: id,
          description: "",
          is_deployed: false,
        };
      });
    },

    openDependencyModal() {
      if (!this.canSetSpaceDependencies) {
        return;
      }
      this.dependencyModal.show = true;
      this.dependencyModal.selectedId = "";
      this.dependencyModal.selectedName = "";
      this.dependencyModal.invalid = false;
      this.refreshDependencyOptions();
    },

    closeDependencyModal() {
      this.dependencyModal.show = false;
      this.dependencyModal.selectedId = "";
      this.dependencyModal.selectedName = "";
      this.dependencyModal.invalid = false;
    },

    addDependency() {
      if (!this.dependencyModal.selectedId) {
        this.dependencyModal.invalid = true;
        return;
      }

      if (!this.formData.depends_on.includes(this.dependencyModal.selectedId)) {
        this.formData.depends_on.push(this.dependencyModal.selectedId);
      }

      this.closeDependencyModal();
      this.refreshDependencyOptions();
    },

    removeDependency(id) {
      this.formData.depends_on = this.formData.depends_on.filter(
        (dependencyId) => dependencyId !== id,
      );
      this.refreshDependencyOptions();
    },

    openRemoveDependencyConfirm(dependency) {
      if (!this.canSetSpaceDependencies) {
        return;
      }
      this.dependencyRemoveConfirm.show = true;
      this.dependencyRemoveConfirm.id = dependency.id;
      this.dependencyRemoveConfirm.name = dependency.name;
    },

    closeRemoveDependencyConfirm() {
      this.dependencyRemoveConfirm.show = false;
      this.dependencyRemoveConfirm.id = "";
      this.dependencyRemoveConfirm.name = "";
    },

    confirmRemoveDependency() {
      if (this.dependencyRemoveConfirm.id) {
        this.removeDependency(this.dependencyRemoveConfirm.id);
      }
      this.closeRemoveDependencyConfirm();
    },

    formatCreatedAt() {
      return this.formData.created_at_formatted || "";
    },

    formatNodeLabel(node) {
      return `${node.hostname} (${node.running_spaces} running / ${node.total_spaces} total)`;
    },

    async initData() {
      focus.Element('input[name="name"]');

      // Ensure availableNodes is always an array to prevent Alpine errors
      this.availableNodes = this.availableNodes || [];

      const iconsResponse = await fetch("/api/icons", {
        headers: {
          "Content-Type": "application/json",
        },
      });
      if (iconsResponse.status === 200) {
        const icons = await iconsResponse.json();
        this.iconList.push(...icons);
      }

      // Fetch user's own scripts for the autocompleter
      const scriptsResponse = await fetch(`/api/scripts?user_id=${userId}`, {
        headers: {
          "Content-Type": "application/json",
        },
      });
      if (scriptsResponse.status === 200) {
        const scripts = await scriptsResponse.json();
        this.scriptList = scripts.scripts.filter(
          (s) => s.script_type === "script" && s.active,
        );
      }

      if (isEdit) {
        const spaceResponse = await fetch(`/api/spaces/${spaceId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        });

        if (spaceResponse.status !== 200) {
          window.location.href = "/spaces";
        } else {
          const space = await spaceResponse.json();

          this.formData.name = space.name;
          this.formData.description = space.description;
          this.formData.template_id = this.template_id = space.template_id;
          this.formData.shell = space.shell;
          this.formData.icon_url = space.icon_url;
          this.formData.custom_fields = space.custom_fields;
          this.formData.created_at = space.created_at;
          this.formData.created_at_formatted = space.created_at_formatted;
          this.formData.startup_script_id = space.startup_script_id || "";
          this.formData.depends_on = space.depends_on || [];
          this.dependencyTargetZone = space.zone || "";

          // Refresh the autocompleter to show the selected script
          this.$nextTick(() => {
            this.$dispatch("refresh-autocompleter");
          });

          if (space.user_id !== userId) {
            this.formData.user_id = space.user_id;
            this.forUsername = space.username;
          } else {
            this.formData.user_id = "";
            this.forUsername = "";
          }

          await this.loadDependencyOptions(space.user_id);

          // Set the alt names and mark all as valid
          this.formData.alt_names = space.alt_names ? space.alt_names : [];
          this.altNameValid = [];
          for (let i = 0; i < this.formData.alt_names.length; i++) {
            this.altNameValid.push(true);
          }
        }
      } else {
        this.dependencyTargetZone = "";
        await this.loadDependencyOptions(this.formData.user_id || userId);
      }

      const templatesResponse = await fetch(
        "/api/templates/" + (isEdit ? this.formData.template_id : templateId),
        {
          headers: {
            "Content-Type": "application/json",
          },
        },
      );
      if (templatesResponse.status === 200) {
        this.template = await templatesResponse.json();
      } else {
        // Set a default template to prevent null reference errors
        this.template = { platform: "manual", custom_fields: [] };
      }

      // Initialize custom fields array immediately to prevent Alpine errors
      if (
        this.template.custom_fields &&
        this.template.custom_fields.length > 0
      ) {
        // If editing, preserve existing values; if creating, initialize with empty strings
        const existingFields = isEdit ? this.formData.custom_fields : [];
        this.formData.custom_fields = this.template.custom_fields.map(
          (field) => {
            return {
              name: field.name,
              value:
                existingFields.find((f) => f.name === field.name)?.value || "",
            };
          },
        );
      } else {
        this.formData.custom_fields = [];
      }

      // Get if the template is manual
      this.isManual = this.template
        ? this.template.platform === "manual"
        : false;
      this.startOnCreate = !this.isManual;

      if (!isEdit) {
        this.formData.icon_url = this.template.icon_url;
      }

      // Fetch available nodes for local container templates
      if (
        !isEdit &&
        this.template &&
        this.template.platform !== "manual" &&
        this.template.platform !== "nomad"
      ) {
        this.loadingNodes = true;
        const nodesResponse = await fetch(
          "/api/templates/" + templateId + "/nodes",
          {
            headers: {
              "Content-Type": "application/json",
            },
          },
        );
        if (nodesResponse.status === 200) {
          const nodes = await nodesResponse.json();
          this.availableNodes = nodes || [];
          if (this.availableNodes.length === 1) {
            this.formData.selected_node_id = this.availableNodes[0].node_id;
          }
        } else {
          // Ensure availableNodes is an empty array on error
          this.availableNodes = [];
        }
        this.loadingNodes = false;
      }

      let darkMode = this.darkMode;
      if (darkMode == null) darkMode = true;

      const editorDesc = ace.edit("description");
      editorDesc.session.setValue(this.formData.description);
      editorDesc.session.on("change", () => {
        this.formData.description = editorDesc.getValue();
      });
      editorDesc.setTheme(
        darkMode ? "ace/theme/github_dark" : "ace/theme/github",
      );
      editorDesc.session.setMode("ace/mode/text");
      editorDesc.setOptions({
        printMargin: false,
        newLineMode: "unix",
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      this.loading = false;
    },
    addAltName() {
      this.altNameValid.push(true);
      this.formData.alt_names.push("");
    },
    removeAltName(index) {
      this.formData.alt_names.splice(index, 1);
      this.altNameValid.splice(index, 1);
    },
    checkName() {
      this.nameValid = validate.name(this.formData.name);
      return this.nameValid;
    },
    checkAltName(index) {
      if (index >= 0 && index < this.formData.alt_names.length) {
        let isValid =
          validate.name(this.formData.alt_names[index]) &&
          this.formData.alt_names[index] !== this.formData.name;

        // If valid then check for duplicate extra name
        if (isValid) {
          for (let i = 0; i < this.formData.alt_names.length; i++) {
            if (
              i !== index &&
              this.formData.alt_names[i] === this.formData.alt_names[index]
            ) {
              isValid = false;
              break;
            }
          }
        }

        this.altNameValid[index] = isValid;
        return isValid;
      } else {
        return false;
      }
    },
    checkDesc() {
      this.descValid = this.formData.description.length <= 1024;
      return this.descValid;
    },
    submitData() {
      let err = false;
      const self = this;

      self.saving = true;
      self.stayOnPage = false;
      err = !this.checkName() || err;
      err = !this.checkDesc() || err;

      // Remove the blank alt names
      for (let i = this.formData.alt_names.length - 1; i >= 0; i--) {
        if (this.formData.alt_names[i] === "") {
          this.formData.alt_names.splice(i, 1);
          this.altNameValid.splice(i, 1);
        }
      }

      // Check the alt names
      for (let i = 0; i < this.formData.alt_names.length; i++) {
        err = !this.checkAltName(i) || err;
      }

      if (err) {
        self.saving = false;
        return;
      }

      this.loading = true;

      fetch(isEdit ? `/api/spaces/${spaceId}` : "/api/spaces", {
        method: isEdit ? "PUT" : "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify(this.formData),
      })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch("show-alert", {
              msg: "Space updated",
              type: "success",
            });
            self.$dispatch("close-space-form");
          } else if (response.status === 201) {
            response.json().then((data) => {
              self.$dispatch("space-created", { space_id: data.space_id });
              self.$dispatch("close-space-form");

              // If start on create
              if (this.startOnCreate) {
                fetch(`/api/spaces/${data.space_id}/start`, {
                  method: "POST",
                  headers: {
                    "Content-Type": "application/json",
                  },
                })
                  .then((response2) => {
                    if (response2.status === 200) {
                      window.dispatchEvent(
                        new CustomEvent("show-alert", {
                          detail: { msg: "Space started", type: "success" },
                        }),
                      );
                    } else {
                      response2.text().then((text) => {
                        try {
                          const d = JSON.parse(text);
                          window.dispatchEvent(
                            new CustomEvent("show-alert", {
                              detail: {
                                msg: `Failed to start space, ${d.error}`,
                                type: "error",
                              },
                            }),
                          );
                        } catch {
                          window.dispatchEvent(
                            new CustomEvent("show-alert", {
                              detail: {
                                msg: `Failed to start space`,
                                type: "error",
                              },
                            }),
                          );
                        }
                      });
                    }
                  })
                  .catch((error) => {
                    window.dispatchEvent(
                      new CustomEvent("show-alert", {
                        detail: {
                          msg: `Error!<br />${error.message}`,
                          type: "error",
                        },
                      }),
                    );
                  });
              }
            });
          } else if (response.status === 507) {
            self.quotaStorageLimitShow = true;
          } else {
            response.json().then((data) => {
              self.$dispatch("show-alert", {
                msg:
                  (isEdit
                    ? "Failed to update space, "
                    : "Failed to create space, ") + data.error,
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

      self.saving = false;
    },
  };
};
