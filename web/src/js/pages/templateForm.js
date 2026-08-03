import { validate } from "../validators.js";
import { focus } from "../focus.js";

/// wysiwyg editor
import ace from "ace-builds/src-noconflict/ace";
import "ace-builds/src-noconflict/mode-terraform";
import "ace-builds/src-noconflict/mode-yaml";
import "ace-builds/src-noconflict/mode-python";
import "ace-builds/src-noconflict/mode-text";
import "ace-builds/src-noconflict/theme-github";
import "ace-builds/src-noconflict/theme-github_dark";
import "ace-builds/src-noconflict/ext-searchbox";
import "ace-builds/src-noconflict/ext-language_tools";
import "ace-builds/src-noconflict/snippets/python";
import "./aceEditorCompleter.js";
import { setSpecCompleter } from "./aceSpecCompleter.js";
import {
  containerSpecCompletions,
  localVolumeSpecCompletions,
  nomadJobCompletions,
  nomadVolumeSpecCompletions,
  templateVariableCompletions,
} from "./specCompletions.js";
import { scriptLibraries } from "./scriptCompletions.js";

window.templateForm = function (isEdit, templateId, isDuplicate = false) {
  return {
    iconList: [],
    scriptList: [],
    templateId: templateId,
    formData: {
      name: "",
      description: "",
      job: "",
      volumes: "",
      groups: [],
      zones: [],
      custom_fields: [],
      ports: [],
      platform: "nomad",
      with_terminal: false,
      with_vscode_tunnel: false,
      with_code_server: false,
      with_ssh: false,
      with_run_command: false,
      allow_node_migration: false,
      startup_script_id: "",
      shutdown_script_id: "",
      compute_units: 0,
      storage_units: 0,
      active: true,
      max_uptime: 0,
      max_uptime_unit: "disabled",
      schedule_enabled: false,
      auto_start: false,
      is_managed: false,
      icon_url: "",
      health_check_type: "none",
      health_check_config: "",
      health_check_skip_ssl_verify: false,
      health_check_timeout: 10,
      health_check_interval: 30,
      health_check_max_failures: 3,
      health_check_auto_restart: false,
      schedule: [
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
        {
          enabled: false,
          from: "12:00am",
          to: "11:59pm",
        },
      ],
    },
    loading: true,
    isEdit: isEdit,
    _formDirty: false,
    discardConfirm: { show: false },
    nameValid: true,
    jobValid: true,
    jobRequired: false,
    volValid: true,
    volRequired: false,
    computeUnitsValid: true,
    storageUnitsValid: true,
    uptimeValid: true,
    groups: [],
    fromHours: [],
    toHours: [],
    zoneValid: [],
    customFieldValid: [],
    showPlatformWarning: false,
    specErrors: {
      job: [],
      volumes: [],
    },
    jobEditor: null,
    volumeEditor: null,

    // Advanced/Wizard toggle — controls whether the raw textareas or the
    // wizard fieldsets are shown for the job + volumes fields.
    // advancedMode: true = show raw editors, false = show wizard.
    // advancedForced: true = the toggle is locked to Advanced because the spec
    // contains constructs the wizard can't display. Derived from parse response.
    advancedMode: true,
    advancedForced: false,
    advancedForcedReason: "",

    async initData() {
      focus.Element('input[name="name"]');

      const iconsResponse = await fetch("/api/icons", {
        headers: {
          "Content-Type": "application/json",
        },
      });
      if (iconsResponse.status === 200) {
        const icons = await iconsResponse.json();
        this.iconList.push(...icons);
      }

      const scriptsResponse = await fetch("/api/scripts/global", {
        headers: {
          "Content-Type": "application/json",
        },
      });
      if (scriptsResponse.status === 200) {
        const scripts = await scriptsResponse.json();
        // Filter for active scripts of type 'script'
        this.scriptList = scripts.scripts.filter(
          (s) => s.script_type === "script" && s.active,
        );
      }

      for (let hour = 0; hour < 24; hour++) {
        for (let minute = 0; minute < 60; minute += 15) {
          const period = hour < 12 || hour === 24 ? "am" : "pm";
          const displayHour = hour % 12 === 0 ? 12 : hour % 12;
          const displayMinute = minute === 0 ? "00" : minute;
          this.fromHours.push(`${displayHour}:${displayMinute}${period}`);
          this.toHours.push(`${displayHour}:${displayMinute}${period}`);
        }
      }
      this.toHours.push("11:59pm");

      const groupsResponse = await fetch("/api/groups", {
        headers: {
          "Content-Type": "application/json",
        },
      });
      const groupsList = await groupsResponse.json();
      this.groups = groupsList.groups;

      if (isEdit) {
        const templateResponse = await fetch(`/api/templates/${templateId}`, {
          headers: {
            "Content-Type": "application/json",
          },
        });

        if (templateResponse.status !== 200) {
          window.location.href = "/spaces";
        } else {
          const template = await templateResponse.json();

          this.formData.name = template.name;
          this.formData.description = template.description;
          this.formData.job = template.job;
          this.formData.volumes = template.volumes;
          this.formData.groups = template.groups;
          this.formData.platform = template.platform;
          this.formData.with_terminal = template.with_terminal;
          this.formData.with_vscode_tunnel = template.with_vscode_tunnel;
          this.formData.with_code_server = template.with_code_server;
          this.formData.with_ssh = template.with_ssh;
          this.formData.with_run_command = template.with_run_command;
          this.formData.allow_node_migration =
            template.allow_node_migration || false;
          this.formData.compute_units = template.compute_units;
          this.formData.storage_units = template.storage_units;
          this.formData.active = template.active;
          this.formData.schedule_enabled = template.schedule_enabled;
          this.formData.auto_start = template.auto_start;
          this.formData.schedule = template.schedule;
          this.formData.max_uptime = template.max_uptime;
          this.formData.max_uptime_unit = template.max_uptime_unit;
          this.formData.icon_url = template.icon_url;
          this.formData.custom_fields = template.custom_fields;
          this.formData.ports = template.ports || [];
          this.formData.startup_script_id = template.startup_script_id || "";
          this.formData.shutdown_script_id = template.shutdown_script_id || "";
          this.formData.is_managed = template.is_managed || false;
          this.formData.health_check_type = template.health_check_type || "none";
          this.formData.health_check_config = template.health_check_config || "";
          this.formData.health_check_skip_ssl_verify = template.health_check_skip_ssl_verify || false;
          this.formData.health_check_timeout = template.health_check_timeout || 10;
          this.formData.health_check_interval = template.health_check_interval || 30;
          this.formData.health_check_max_failures = template.health_check_max_failures || 3;
          this.formData.health_check_auto_restart = template.health_check_auto_restart || false;

          // Set the zones and mark all as valid
          this.formData.zones = template.zones ? template.zones : [];
          this.zoneValid = [];
          this.formData.zones.forEach(() => {
            this.zoneValid.push(true);
          });
          this.customFieldValid = [];
          this.formData.custom_fields.forEach(() => {
            this.customFieldValid.push(true);
          });

          this.$nextTick(() => {
            this.$dispatch("refresh-autocompleter");
          });
        }

        // If this is duplicate then change to create mode
        if (isDuplicate) {
          this.formData.name = `Copy of ${this.formData.name}`;
          this.isEdit = isEdit = false;
        }
      }

      let darkMode = this.darkMode;
      if (darkMode == null) darkMode = true;

      // Create the job editor
      const editor = ace.edit("job");
      this.jobEditor = editor;
      editor.session.setValue(this.formData.job);
      editor.session.on("change", () => {
        this.formData.job = editor.getValue();
        this.clearSpecFieldErrors("job");
        this.scheduleSpecValidation();
        this._formDirty = true;
      });
      editor.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editor.setOptions({
        printMargin: false,
        newLineMode: "unix",
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      // Create the volume editor
      const editorVol = ace.edit("vol");
      this.volumeEditor = editorVol;
      editorVol.session.setValue(this.formData.volumes);
      editorVol.session.on("change", () => {
        this.formData.volumes = editorVol.getValue();
        this.clearSpecFieldErrors("volumes");
        this.scheduleSpecValidation();
        this._formDirty = true;
      });
      editorVol.setTheme(
        darkMode ? "ace/theme/github_dark" : "ace/theme/github",
      );
      editorVol.setOptions({
        printMargin: false,
        newLineMode: "unix",
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      // Create the description editor
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

      // Create the health check script editor
      const editorHC = ace.edit("health_check_script");
      editorHC.session.setValue(this.formData.health_check_config);
      editorHC.session.on("change", () => {
        if (this.formData.health_check_type === "custom") {
          this.formData.health_check_config = editorHC.getValue();
        }
      });
      editorHC.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editorHC.session.setMode("ace/mode/python");
      window.AceEditorCompleter.setup(editorHC, scriptLibraries, { debug: false });
      editorHC.setOptions({
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
      this.$watch("formData.health_check_type", (val) => {
        if (val === "custom") {
          this.$nextTick(() => editorHC.resize());
        }
      });

      this.$watch("formData.platform", () => {
        this.applySpecEditors();
        this.specErrors.job = [];
        this.specErrors.volumes = [];
        this.jobValid = true;
        this.volValid = true;
      });

      this.applySpecEditors();

      window.addEventListener("theme-change", (e) => {
        if (e.detail.dark_theme) {
          editor.setTheme("ace/theme/github_dark");
          editorVol.setTheme("ace/theme/github_dark");
          editorDesc.setTheme("ace/theme/github_dark");
          editorHC.setTheme("ace/theme/github_dark");
        } else {
          editor.setTheme("ace/theme/github");
          editorVol.setTheme("ace/theme/github");
          editorDesc.setTheme("ace/theme/github");
          editorHC.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    toggleGroup(groupId) {
      if (this.formData.groups.includes(groupId)) {
        const index = this.formData.groups.indexOf(groupId);
        this.formData.groups.splice(index, 1);
      } else {
        this.formData.groups.push(groupId);
      }
    },
    toggleDaySchedule(day) {
      this.formData.schedule[day].enabled =
        !this.formData.schedule[day].enabled;
    },
    checkPlatform() {
      return validate.isOneOf(this.formData.platform, [
        "manual",
        "docker",
        "podman",
        "nomad",
        "apple",
        "container",
      ]);
    },
    checkName() {
      this.nameValid = validate.templateName(this.formData.name);
      return this.nameValid;
    },
    checkJob() {
      this.jobRequired =
        this.formData.platform !== "manual" &&
        !validate.required(this.formData.job);
      this.jobValid = !this.jobRequired;
      return this.jobValid;
    },
    checkComputeUnits() {
      this.computeUnitsValid = validate.isNumber(
        this.formData.compute_units,
        0,
        Infinity,
      );
      return this.computeUnitsValid;
    },
    checkStorageUnits() {
      this.storageUnitsValid = validate.isNumber(
        this.formData.storage_units,
        0,
        Infinity,
      );
      return this.storageUnitsValid;
    },
    checkUptime() {
      if (this.formData.max_uptime_unit === "disabled") {
        this.uptimeValid = true;
      } else {
        this.uptimeValid =
          validate.isNumber(this.formData.max_uptime, 0, Infinity) &&
          validate.isOneOf(this.formData.max_uptime_unit, [
            "minute",
            "hour",
            "day",
          ]);
      }
      return this.uptimeValid;
    },
    checkZonesValid() {
      let zonesValid = true;
      this.formData.zones.forEach((zone, index) => {
        zonesValid = zonesValid && this.zoneValid[index];
      });
      return zonesValid;
    },
    checkCustomFieldsValid() {
      let fieldsValid = true;
      this.formData.custom_fields.forEach((field, index) => {
        fieldsValid = fieldsValid && this.customFieldValid[index];
      });
      return fieldsValid;
    },
    applySpecEditors() {
      if (!this.jobEditor || !this.volumeEditor) {
        return;
      }

      this.jobEditor.session.setMode(
        this.isLocalContainer() ? "ace/mode/yaml" : "ace/mode/terraform",
      );
      this.volumeEditor.session.setMode("ace/mode/yaml");

      setSpecCompleter(
        this.jobEditor,
        [
          ...(this.isLocalContainer()
            ? containerSpecCompletions
            : nomadJobCompletions),
          ...templateVariableCompletions,
        ],
      );
      setSpecCompleter(
        this.volumeEditor,
        [
          ...(this.isLocalContainer()
            ? localVolumeSpecCompletions
            : nomadVolumeSpecCompletions),
          ...templateVariableCompletions,
        ],
      );
    },
    clearSpecFieldErrors(field) {
      if (field === "job") {
        this.specErrors.job = [];
        this.jobValid = !this.jobRequired;
        if (this.jobEditor) {
          this.jobEditor.session.clearAnnotations();
        }
      }

      if (field === "volumes") {
        this.specErrors.volumes = [];
        this.volValid = !this.volRequired;
        if (this.volumeEditor) {
          this.volumeEditor.session.clearAnnotations();
        }
      }
    },
    setEditorErrors(editor, errors) {
      if (!editor) {
        return;
      }

      editor.session.setAnnotations(
        errors.map((entry) => {
          const message =
            typeof entry === "string" ? entry : entry.message;
          let row = 0;
          if (typeof entry === "object" && entry.line) {
            row = entry.line - 1; // Issue.Line is 1-based; ace rows are 0-based
          } else {
            const m = message.match(/line (\d+)/i);
            if (m) row = parseInt(m[1], 10) - 1;
          }
          if (row < 0) row = 0;
          return { row, column: 0, text: message, type: "error" };
        }),
      );
    },
    scheduleSpecValidation() {
      clearTimeout(this._specValidationTimer);
      this._specValidationTimer = setTimeout(() => {
        this.validateSpecs();
      }, 750);
    },

    async validateSpecs() {
      if (this.formData.platform === "manual") {
        this.clearSpecFieldErrors("job");
        this.clearSpecFieldErrors("volumes");
        return true;
      }

      const response = await fetch("/api/templates/validate", {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
        },
        body: JSON.stringify({
          platform: this.formData.platform,
          job: this.formData.job,
          volumes: this.formData.volumes,
        }),
      });

      const result = await response.json();
      const errors = result.errors || [];

      const jobErrors = errors.filter((error) => error.field === "job");
      const volErrors = errors.filter((error) => error.field === "volumes");
      this.specErrors.job = jobErrors.map((error) => error.message);
      this.specErrors.volumes = volErrors.map((error) => error.message);

      this.jobValid = !this.jobRequired && this.specErrors.job.length === 0;
      this.volValid = !this.volRequired && this.specErrors.volumes.length === 0;
      this.setEditorErrors(this.jobEditor, jobErrors);
      this.setEditorErrors(this.volumeEditor, volErrors);

      return response.ok && !!result.valid;
    },

    async submitData(continueEditing = false) {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkJob() || err;
      err = !this.checkPlatform() || err;
      err = !this.checkZonesValid() || err;
      err = !this.checkCustomFieldsValid() || err;
      if (err) {
        this.$dispatch("show-alert", {
          msg: "Please fix the validation errors before saving",
          type: "error",
        });
        return;
      }

      try {
        const specsValid = await this.validateSpecs();
        if (!specsValid) {
          this.$dispatch("show-alert", {
            msg: "Please fix the spec validation errors before saving",
            type: "error",
          });
          return;
        }
      } catch (error) {
        self.$dispatch("show-alert", {
          msg: `Failed to validate the template, ${error.message}`,
          type: "error",
        });
        return;
      }

      if (!continueEditing) {
        this.loading = true;
      }

      const data = {
        name: this.formData.name,
        description: this.formData.description,
        job: this.formData.platform === "manual" ? "" : this.formData.job,
        volumes:
          this.formData.platform === "manual" ? "" : this.formData.volumes,
        groups: this.formData.groups,
        with_terminal: this.formData.with_terminal,
        with_vscode_tunnel: this.formData.with_vscode_tunnel,
        with_code_server: this.formData.with_code_server,
        with_ssh: this.formData.with_ssh,
        with_run_command: this.formData.with_run_command,
        allow_node_migration: this.isLocalContainer()
          ? this.formData.allow_node_migration
          : false,
        startup_script_id:
          this.formData.platform === "manual"
            ? ""
            : this.formData.startup_script_id,
        shutdown_script_id:
          this.formData.platform === "manual"
            ? ""
            : this.formData.shutdown_script_id,
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
        schedule_enabled:
          this.formData.schedule_enabled && this.formData.platform !== "manual",
        auto_start: this.formData.auto_start,
        schedule: this.formData.schedule,
        zones: this.formData.zones,
        active: this.formData.active,
        max_uptime: parseInt(this.formData.max_uptime),
        max_uptime_unit:
          this.formData.platform !== "manual"
            ? "disabled"
            : this.formData.max_uptime_unit,
        platform: this.formData.platform,
        icon_url: this.formData.icon_url,
        custom_fields: this.formData.custom_fields,
        ports: this.formData.ports,
        health_check_type: this.formData.platform === "manual" ? "none" : this.formData.health_check_type,
        health_check_config: ["none", "agent"].includes(this.formData.health_check_type) ? "" : this.formData.health_check_config,
        health_check_skip_ssl_verify: this.formData.health_check_skip_ssl_verify,
        health_check_timeout: parseInt(this.formData.health_check_timeout) || 10,
        health_check_interval: parseInt(this.formData.health_check_interval) || 30,
        health_check_max_failures: parseInt(this.formData.health_check_max_failures) || 3,
        health_check_auto_restart: this.formData.health_check_auto_restart,
      };

      await fetch(
        this.isEdit ? `/api/templates/${this.templateId}` : "/api/templates",
        {
          method: this.isEdit ? "PUT" : "POST",
          headers: {
            "Content-Type": "application/json",
          },
          body: JSON.stringify(data),
        },
      )
        .then(async (response) => {
          if (response.status === 200) {
            self._formDirty = false;
            self.$dispatch("show-alert", {
              msg: "Template updated",
              type: "success",
            });

            if (!continueEditing) {
              self.$dispatch("close-template-form");
            }
          } else if (response.status === 201) {
            const data = await response.json();

            self._formDirty = false;
            self.$dispatch("show-alert", {
              msg: "Template created",
              type: "success",
            });

            if (continueEditing) {
              this.templateId = data.template_id;
              this.isEdit = true;
            } else {
              self.$dispatch("close-template-form");
            }
          } else {
            response.json().then((d) => {
              self.$dispatch("show-alert", {
                msg: `Failed to update the template, ${d.error}`,
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
    getDayOfWeek(day) {
      return [
        "Sunday",
        "Monday",
        "Tuesday",
        "Wednesday",
        "Thursday",
        "Friday",
        "Saturday",
      ][day];
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
        let isValid = validate.maxLength(this.formData.zones[index], 64);

        // If valid then check for duplicate extra name
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
      } else {
        return false;
      }
    },
    addField() {
      this.customFieldValid.push(true);
      this.formData.custom_fields.push({ name: "", description: "" });
    },
    removeField(index) {
      this.formData.custom_fields.splice(index, 1);
      this.customFieldValid.splice(index, 1);
    },
    checkCustomField(index) {
      if (index >= 0 && index < this.formData.custom_fields.length) {
        let isValid =
          validate.maxLength(this.formData.custom_fields[index].name, 24) &&
          validate.varName(this.formData.custom_fields[index].name) &&
          validate.maxLength(
            this.formData.custom_fields[index].description,
            256,
          );

        // If valid then check for duplicate name
        if (isValid) {
          for (let i = 0; i < this.formData.custom_fields.length; i++) {
            if (
              i !== index &&
              this.formData.custom_fields[i].name ===
                this.formData.custom_fields[index].name
            ) {
              isValid = false;
              break;
            }
          }
        }

        this.customFieldValid[index] = isValid;
        return isValid;
      } else {
        return false;
      }
    },
    addPort() {
      this.formData.ports.push({ name: "", port: 0, protocol: "http" });
    },
    removePort(index) {
      this.formData.ports.splice(index, 1);
    },
    checkPort(index) {
      if (index >= 0 && index < this.formData.ports.length) {
        const name = this.formData.ports[index].name;
        const port = this.formData.ports[index].port;
        const isValid =
          name.length > 0 &&
          !name.includes('=') &&
          !name.includes(',') &&
          validate.isNumber(port, 1, 65535);
        return isValid;
      } else {
        return false;
      }
    },
    isLocalContainer() {
      return (
        this.formData.platform === "docker" ||
        this.formData.platform === "podman" ||
        this.formData.platform === "apple" ||
        this.formData.platform === "container"
      );
    },

    // ── Template spec wizard ────────────────────────────────────────────
    //
    // State and handlers for the "Build spec…" wizard. The wizard is a full
    // modal overlay that blocks the underlying Job/Volumes textareas; on Apply
    // it patches the wizard-controlled fields back into the ace editors via
    // /api/spec/build.
    //
    // Icon flow (per design):
    //   - On open, wizard.icon is cleared.
    //   - Selecting a base image via the picker writes both image string and
    //     the manifest icon URL into wizard.icon.
    //   - Typing in the image field does NOT clear wizard.icon.
    //   - On Apply, if wizard.icon is set, it's written to the template's
    //     icon_url; if empty, the template's icon_url is left untouched.
    specWizard: {
      show: false,
      loading: false,
      saving: false,
      error: "",
      wizardable: true,
      notWizardableReason: "",
      baseImages: [],
      baseImagesLoaded: false,
      baseImagesError: "",
      registryAuth: false,
      imageSearch: "",
      imageDropdownOpen: false,
      imageActiveIndex: 0,
      // Linux capability catalog (name + description) for the searchable
      // picker, plus per-list dropdown state. Only cap_add is editable in the
      // wizard; cap_drop is kept in state so a spec that already drops
      // capabilities round-trips untouched, and is edited in the raw spec.
      capabilities: [],
      capabilitiesLoaded: false,
      capabilitiesError: "",
      capPicker: {
        cap_add: { open: false, search: "" },
        cap_drop: { open: false, search: "" },
      },
      // Volume details modal state (Nomad only)
      volumeDetails: {
        show: false,
        index: -1,
        parameters: "",
        secrets: "",
        capabilities: "",
        mountOptions: "",
      },
      icon: "", // hidden — only set by picker; cleared on open
      spec: {
        name: "",
        image: "",
        hostname: "",
        command: [],
        environment: [],
        ports: [],
        storage: [],
        devices: [],
        extra_hosts: [],
        dns: [],
        dns_search: [],
        cap_add: [],
        cap_drop: [],
        network: "",
        privileged: false,
        memory: "",
        cpus: "",
        cpu_type: "",
        auth: null,
        templates: [],
      },

      templateEditor: {
        show: false,
        index: -1,
        destination: "",
        change_mode: "",
        change_signal: "",
        mount_target: "",
        mount_readonly: false,
        ace: null,
      },
    },

    async openSpecWizard() {
      this.specWizard.show = true;
      this.specWizard.loading = true;
      this.specWizard.error = "";
      this.specWizard.icon = ""; // cleared on every open per design

      this.specWizard.capPicker.cap_add = { open: false, search: "" };
      this.specWizard.capPicker.cap_drop = { open: false, search: "" };

      if (!this.specWizard.baseImagesLoaded) {
        await this.loadBaseImages();
      }
      if (!this.specWizard.capabilitiesLoaded) {
        await this.loadCapabilities();
      }

      // Parse the current spec via the server so values populate the wizard.
      const platform = this.formData.platform;
      const job = this.jobEditor ? this.jobEditor.getValue() : this.formData.job;
      const volumes = this.volumeEditor ? this.volumeEditor.getValue() : this.formData.volumes;

      try {
        const resp = await fetch("/api/spec/parse", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ platform, job, volumes }),
        });
        if (resp.status === 401) {
          window.location.href = "/logout";
          return;
        }
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status}`);
        }
        const data = await resp.json();
        this.specWizard.wizardable = !!data.wizardable;
        this.specWizard.notWizardableReason = data.reason || "";
        if (data.wizardable && data.spec) {
          this.specWizard.spec = this.normaliseSpec(data.spec);
          // Default hostname/name for new templates (empty job) — matches the
          // convention used by all stock knot-base-images specs.
          if (!this.specWizard.spec.hostname && !job.trim()) {
            this.specWizard.spec.hostname = "${{ .space.name }}";
          }
          if (!this.specWizard.spec.name && !job.trim()) {
            this.specWizard.spec.name = "${{ .user.username }}-${{ .space.name }}";
          }
        } else {
          // Keep an empty spec visible so the UI doesn't crash; Apply stays
          // disabled while wizardable is false.
          this.specWizard.spec = this.normaliseSpec({});
        }
      } catch (err) {
        this.specWizard.error = "Failed to parse current spec: " + err.message;
        this.specWizard.wizardable = false;
      } finally {
        this.specWizard.loading = false;
      }
    },

    async loadBaseImages() {
      try {
        const resp = await fetch("/api/base-images", {
          headers: { "Content-Type": "application/json" },
        });
        if (resp.status === 401) {
          window.location.href = "/logout";
          return;
        }
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status}`);
        }
        const data = await resp.json();
        this.specWizard.baseImages = (data.images || []).filter((i) => i.image);
        this.specWizard.registryAuth = !!data.registry_auth;
        this.specWizard.baseImagesLoaded = true;
      } catch (err) {
        this.specWizard.baseImagesError = "Failed to load base images: " + err.message;
      }
    },

    async loadCapabilities() {
      try {
        const resp = await fetch("/api/capabilities", {
          headers: { "Content-Type": "application/json" },
        });
        if (resp.status === 401) {
          window.location.href = "/logout";
          return;
        }
        if (!resp.ok) {
          throw new Error(`HTTP ${resp.status}`);
        }
        const data = await resp.json();
        this.specWizard.capabilities = data.capabilities || [];
        this.specWizard.capabilitiesLoaded = true;
      } catch (err) {
        this.specWizard.capabilitiesError =
          "Failed to load capabilities: " + err.message;
      }
    },

    // Canonical form is CAP_UPPER_SNAKE, matching the server. Returns "" for
    // anything that isn't a usable capability name.
    normaliseCapability(name) {
      const n = (name || "").trim().toUpperCase();
      if (!n) return "";
      const body = n.startsWith("CAP_") ? n.slice(4) : n;
      if (!body || !/^[A-Z][A-Z0-9_]*$/.test(body)) return "";
      return "CAP_" + body;
    },

    // Search matches the capability name first, then the description, so
    // "net_admin" and "network administration" both find CAP_NET_ADMIN. Every
    // whitespace-separated word must match somewhere, which keeps multi-word
    // searches like "raw sockets" useful. Already selected capabilities are
    // filtered out of their own list.
    filteredCapabilities(kind) {
      const words = (this.specWizard.capPicker[kind].search || "")
        .trim()
        .toLowerCase()
        .split(/\s+/)
        .filter((w) => w.length > 0);
      const selected = new Set(this.specWizard.spec[kind] || []);
      const scored = [];
      for (const cap of this.specWizard.capabilities) {
        if (selected.has(cap.name)) continue;
        const name = (cap.name || "").toLowerCase();
        const haystack = name + " " + (cap.description || "").toLowerCase();
        let rank;
        if (words.length === 0) {
          rank = 0;
        } else if (words.every((w) => name.includes(w))) {
          rank = 0;
        } else if (words.every((w) => haystack.includes(w))) {
          rank = 1;
        } else {
          continue;
        }
        scored.push({ cap, rank });
      }
      const coll = new Intl.Collator(undefined, { sensitivity: "base" });
      scored.sort((a, b) => {
        if (a.rank !== b.rank) return a.rank - b.rank;
        if (!!a.cap.common !== !!b.cap.common) return a.cap.common ? -1 : 1;
        return coll.compare(a.cap.name, b.cap.name);
      });
      return scored.map((s) => s.cap);
    },

    // Description shown under a selected pill; blank for names that aren't in
    // the catalog (hand-written capabilities still round-trip fine).
    capabilityDescription(name) {
      const match = this.specWizard.capabilities.find((c) => c.name === name);
      return match ? match.description : "";
    },

    addCapability(kind, name) {
      const canonical = this.normaliseCapability(name);
      if (!canonical) return false;
      this.specWizard.capabilitiesError = "";
      if (!Array.isArray(this.specWizard.spec[kind])) {
        this.specWizard.spec[kind] = [];
      }
      if (!this.specWizard.spec[kind].includes(canonical)) {
        this.specWizard.spec[kind].push(canonical);
      }
      this.specWizard.capPicker[kind].search = "";
      this.specWizard.capPicker[kind].open = false;
      return true;
    },

    // Enter in the search box adds whatever was typed, so capabilities missing
    // from the catalog can still be selected.
    addTypedCapability(kind) {
      const typed = this.specWizard.capPicker[kind].search;
      const matches = this.filteredCapabilities(kind);
      if (matches.length > 0) {
        this.addCapability(kind, matches[0].name);
        return;
      }
      if (!this.addCapability(kind, typed)) {
        this.specWizard.capabilitiesError = `"${typed}" isn't a valid capability name.`;
      }
    },

    removeCapability(kind, index) {
      this.specWizard.spec[kind].splice(index, 1);
    },

    toggleCapabilityPicker(kind) {
      const picker = this.specWizard.capPicker[kind];
      picker.open = !picker.open;
      if (picker.open) {
        picker.search = "";
        this.specWizard.capPicker[kind === "cap_add" ? "cap_drop" : "cap_add"].open = false;
      }
    },

    filteredBaseImages() {
      const term = (this.specWizard.imageSearch || "").toLowerCase();
      let list = this.specWizard.baseImages;
      if (term) {
        list = list.filter(
          (i) =>
            (i.display_name || "").toLowerCase().includes(term) ||
            (i.description || "").toLowerCase().includes(term) ||
            (i.name || "").toLowerCase().includes(term) ||
            (i.category || "").toLowerCase().includes(term),
        );
      }
      // Recommended first, each bucket sorted by display_name (case-insensitive).
      const coll = new Intl.Collator(undefined, { sensitivity: "base" });
      return [...list].sort((a, b) => {
        if (!!a.recommended !== !!b.recommended) return a.recommended ? -1 : 1;
        return coll.compare(a.display_name || "", b.display_name || "");
      });
    },

    // Keyboard navigation for the base-image picker: ↑/↓ move the active row,
    // Enter selects it. Active row is also tracked on mouse move so hover and
    // keyboard stay in sync.
    imageMoveDown() {
      const n = this.filteredBaseImages().length;
      if (n === 0) return;
      this.specWizard.imageActiveIndex = Math.min(this.specWizard.imageActiveIndex + 1, n - 1);
      this.scrollImageActiveIntoView();
    },
    imageMoveUp() {
      const n = this.filteredBaseImages().length;
      if (n === 0) return;
      this.specWizard.imageActiveIndex = Math.max(this.specWizard.imageActiveIndex - 1, 0);
      this.scrollImageActiveIntoView();
    },
    imagePickActive() {
      const list = this.filteredBaseImages();
      const i = this.specWizard.imageActiveIndex;
      if (i >= 0 && i < list.length) this.pickBaseImage(list[i]);
    },
    scrollImageActiveIntoView() {
      this.$nextTick(() => {
        const list = this.$refs.imageList;
        if (!list) return;
        const item = list.querySelector('[data-img-idx="' + this.specWizard.imageActiveIndex + '"]');
        if (!item) return;
        const scroller = list.parentElement; // the .overflow-auto container
        if (!scroller) return;
        // The sticky search header sits at the top of the scroller, so the
        // usable top is below it. scrollIntoView can't see sticky elements,
        // so adjust scrollTop manually to keep the active row fully visible.
        const header = scroller.firstElementChild;
        const headerH = header ? header.offsetHeight : 0;
        const itemRect = item.getBoundingClientRect();
        const scrollerRect = scroller.getBoundingClientRect();
        const topBound = scrollerRect.top + headerH;
        if (itemRect.top < topBound) {
          scroller.scrollTop -= topBound - itemRect.top;
        } else if (itemRect.bottom > scrollerRect.bottom) {
          scroller.scrollTop += itemRect.bottom - scrollerRect.bottom;
        }
      });
    },

    // Called when the user picks a base image from the picker dropdown.
    // Writes both the image string (visible) and the icon URL (hidden).
    pickBaseImage(image) {
      this.specWizard.spec.image = image.image;
      this.specWizard.icon = image.icon || "";
      this.specWizard.imageSearch = "";
      this.specWizard.imageDropdownOpen = false;
      // Auto-fill the template name if empty.
      if (!this.formData.name && image.display_name) {
        this.formData.name = image.display_name;
        this.checkName();
      }
      if (image.default_memory && !this.specWizard.spec.memory) {
        this.specWizard.spec.memory = image.default_memory;
      }
      if (image.default_cores && !this.specWizard.spec.cpus) {
        this.specWizard.spec.cpus = image.default_cores;
        this.specWizard.spec.cpu_type = "cores";
      } else if (image.default_cpus && !this.specWizard.spec.cpus) {
        this.specWizard.spec.cpus = image.default_cpus;
      }
      // Image-specific env vars (e.g. KNOT_VNC_HTTP_PORT for desktop images).
      if (image.default_env) {
        for (const entry of image.default_env) {
          const eqIdx = entry.indexOf("=");
          if (eqIdx < 1) continue;
          const key = entry.slice(0, eqIdx);
          const value = entry.slice(eqIdx + 1);
          if (!this.specWizard.spec.environment.some((e) => e.key === key)) {
            this.specWizard.spec.environment.unshift({ key, value });
          }
        }
      }
      // Image-specific template ports (e.g. Web:80:http for PHP images).
      if (image.default_port) {
        for (const dp of image.default_port) {
          if (!this.formData.ports.some((p) => p.port === dp.port)) {
            this.formData.ports.unshift({ name: dp.name, port: dp.port, protocol: dp.protocol });
          }
        }
      }
      // Image-specific storage volumes (e.g. /home, /data, /var/lib/mysql).
      // Pre-fill one row per declared mount point using the manifest's
      // suggested kind (defaulting to "volume"); the user can change the kind
      // (bind/path/volume) or remove it. Skipped if a row for the same
      // container path already exists.
      if (image.volumes && image.volumes.length) {
        for (const v of image.volumes) {
          const path = (v && v.path) || "";
          if (!path) continue;
          if (this.specWizard.spec.storage.some((s) => s.container_path === path)) continue;
          const kind = v.kind === "bind" || v.kind === "path" ? v.kind : "volume";
          const name = path.replace(/^\/*/, "").split("/").filter(Boolean).pop() || "data";
          const entry = {
            kind,
            container_path: path,
            read_only: false,
            host_path: "",
            name,
            size: "",
          };
          if (this.formData.platform === "nomad") {
            entry.plugin_id = "";
            entry.capacity_min = "";
            entry.capacity_max = "";
            entry.parameters = {};
            entry.access_modes = [{ access_mode: "single-node-writer", attachment_mode: "file-system" }];
          }
          this.specWizard.spec.storage.unshift(entry);
        }
      }
      // Inject registry auth from server config if available and the spec
      // doesn't already have an auth block.
      if (this.specWizard.registryAuth && !this.specWizard.spec.auth) {
        this.specWizard.spec.auth = {
          username: "${{ .server.base_image_registry_user }}",
          password: "${{ .server.base_image_registry_password }}",
        };
      }
    },

    // Normalises a parsed spec so the wizard always sees all fields as arrays
    // / objects (never null/undefined). Avoids x-for explosions on empty.
    normaliseSpec(spec) {
      const s = spec || {};
      const toArray = (v) => (Array.isArray(v) ? v : []);
      // Ensure Nomad volume entries always have a parameters object for x-model bindings.
      const normaliseStorage = (arr) => {
        if (!Array.isArray(arr)) return [];
        return arr.map((e) => {
          if (e.kind === "volume" && (!e.parameters || typeof e.parameters !== "object")) {
            e.parameters = {};
          }
          return e;
        });
      };
      return {
        name: s.name || "",
        image: s.image || "",
        hostname: s.hostname || "",
        command: toArray(s.command),
        environment: toArray(s.environment),
        ports: toArray(s.ports),
        storage: normaliseStorage(s.storage),
        devices: toArray(s.devices),
        extra_hosts: toArray(s.extra_hosts),
        dns: toArray(s.dns),
        dns_search: toArray(s.dns_search),
        cap_add: toArray(s.cap_add),
        cap_drop: toArray(s.cap_drop),
        network: s.network || "",
        privileged: !!s.privileged,
        memory: s.memory || "",
        memory_max: s.memory_max || "",
        cpus: s.cpus || "",
        cpu_type: s.cpu_type || "",
        auth: s.auth || null,
        driver: s.driver || (this.formData.platform === "nomad" ? "docker" : ""),
        templates: toArray(s.templates),
      };
    },

    // Wizard list helpers — the array variants need a stable shape for x-for.
    // New entries go at the top so they're visible without scrolling once the
    // list gets long.
    wizardAddEnv() {
      this.specWizard.spec.environment.unshift({ key: "", value: "" });
    },
    wizardRemoveEnv(i) {
      this.specWizard.spec.environment.splice(i, 1);
    },
    wizardAddPort() {
      this.specWizard.spec.ports.push({ label: "", host_port: null, container_port: null, protocol: "tcp" });
    },
    wizardRemovePort(i) {
      this.specWizard.spec.ports.splice(i, 1);
    },
    wizardAddStorage(kind) {
      const entry = { kind: kind || "bind", container_path: "", read_only: false, host_path: "", name: "", size: "" };
      if (this.formData.platform === "nomad" && kind === "volume") {
        entry.plugin_id = "";
        entry.capacity_min = "";
        entry.capacity_max = "";
        entry.parameters = {};
        entry.access_modes = [{ access_mode: "single-node-writer", attachment_mode: "file-system" }];
      }
      this.specWizard.spec.storage.unshift(entry);
    },
    wizardRemoveStorage(i) {
      this.specWizard.spec.storage.splice(i, 1);
    },
    setVolumeType(i, value) {
      const entry = this.specWizard.spec.storage[i];
      if (value === "host") {
        entry.volume_type = "host";
        entry.access_modes = [];
        if (!entry.plugin_id) entry.plugin_id = "mkdir";
      } else {
        entry.volume_type = "csi";
        const mode = value === "multi" ? "multi-node-multi-writer" : "single-node-writer";
        entry.access_modes = [{ access_mode: mode, attachment_mode: "file-system" }];
      }
    },
    // ensureVolumeScaffolding runs after a storage row's kind changes (via the
    // kind <select>). Switching a row TO a managed volume on Nomad needs the
    // CSI/host fields the builder expects; add them only if absent so flipping
    // back and forth doesn't clobber values the user already set. Switching
    // away from volume leaves the fields in place (hidden, ignored by the
    // builder) so the user doesn't lose them.
    ensureVolumeScaffolding(i) {
      const entry = this.specWizard.spec.storage[i];
      if (!entry) return;
      if (entry.kind !== "volume" || this.formData.platform !== "nomad") return;
      if (entry.access_modes == null) {
        entry.access_modes = [{ access_mode: "single-node-writer", attachment_mode: "file-system" }];
      }
      if (entry.parameters == null) entry.parameters = {};
      if (entry.plugin_id == null) entry.plugin_id = "";
      if (entry.capacity_min == null) entry.capacity_min = "";
      if (entry.capacity_max == null) entry.capacity_max = "";
    },

    // --- Volume details modal (Nomad: parameters, secrets, capabilities, mount_options) ---

    openVolumeDetails(index) {
      const entry = this.specWizard.spec.storage[index];
      if (!entry) return;
      const d = this.specWizard.volumeDetails;
      d.index = index;
      // Serialize maps/arrays to YAML text for the textarea editors.
      d.parameters = this._mapToYaml(entry.parameters);
      d.secrets = this._mapToYaml(entry.secrets);
      d.capabilities = this._capabilitiesToYaml(entry.access_modes);
      d.mountOptions = this._mountOptionsToYaml(entry.fs_type, entry.mount_flags);
      d.show = true;
    },

    closeVolumeDetails() {
      this.specWizard.volumeDetails.show = false;
    },

    applyVolumeDetails() {
      const d = this.specWizard.volumeDetails;
      const entry = this.specWizard.spec.storage[d.index];
      if (!entry) { d.show = false; return; }
      // Parse YAML text back into structured fields.
      entry.parameters = this._yamlToMap(d.parameters);
      entry.secrets = this._yamlToMap(d.secrets);
      entry.access_modes = this._yamlToCapabilities(d.capabilities, entry.access_modes);
      const mo = this._yamlToMountOptions(d.mountOptions);
      entry.fs_type = mo.fs_type || "";
      entry.mount_flags = mo.mount_flags || [];
      d.show = false;
    },

    // --- Template editor (Nomad: heredoc data + optional mount) ---

    wizardAddTemplate() {
      this.specWizard.spec.templates.unshift({
        destination: "",
        data: "",
        change_mode: "",
        change_signal: "",
        mount_target: "",
        mount_readonly: false,
      });
      // Open the editor for the freshly-added row (index 0) so the user can
      // start typing without an extra click.
      this.openTemplateEditor(0);
    },

    wizardRemoveTemplate(i) {
      this.specWizard.spec.templates.splice(i, 1);
    },

    openTemplateEditor(i) {
      const entry = this.specWizard.spec.templates[i];
      if (!entry) return;
      const d = this.specWizard.templateEditor;
      d.index = i;
      d.destination = entry.destination || "";
      d.change_mode = entry.change_mode || "";
      d.change_signal = entry.change_signal || "";
      d.mount_target = entry.mount_target || "";
      d.mount_readonly = !!entry.mount_readonly;
      d.show = true;
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          let container = this.$refs.templateAceEditor || document.getElementById("templateAceEditor");
          if (!container) return;
          let darkMode = false;
          try { darkMode = JSON.parse(localStorage.getItem("_x_darkMode")); } catch (e) {}
          // Reuse existing editor if it was already created.
          if (d.ace) {
            d.ace.setValue(entry.data || "");
            d.ace.clearSelection();
            d.ace.moveCursorToPosition({ row: 0, column: 0 });
            d.ace.resize();
            return;
          }
          d.ace = ace.edit(container);
          d.ace.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
          d.ace.session.setMode("ace/mode/text");
          d.ace.setOptions({
            printMargin: false,
            newLineMode: "unix",
            tabSize: 2,
            wrap: true,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
            useWorker: false,
          });
          setSpecCompleter(d.ace, [...templateVariableCompletions]);
          d.ace.setValue(entry.data || "");
          d.ace.clearSelection();
          d.ace.moveCursorToPosition({ row: 0, column: 0 });
          d.ace.resize();
        });
      });
    },

    closeTemplateEditor() {
      this.specWizard.templateEditor.show = false;
    },

    applyTemplateEditor() {
      const d = this.specWizard.templateEditor;
      const entry = this.specWizard.spec.templates[d.index];
      if (!entry) { d.show = false; return; }
      entry.destination = d.destination;
      entry.data = d.ace ? d.ace.getValue() : "";
      entry.change_mode = d.change_mode;
      entry.change_signal = d.change_signal;
      entry.mount_target = d.mount_target;
      entry.mount_readonly = d.mount_readonly;
      d.show = false;
    },

    // Helper: object → YAML lines (flat key-value only)
    _mapToYaml(obj) {
      if (!obj || typeof obj !== "object") return "";
      const lines = [];
      for (const [k, v] of Object.entries(obj)) {
        if (v === "" || v === undefined || v === null) continue;
        // Quote strings that contain special chars; leave numbers unquoted.
        const val = typeof v === "number" ? String(v) : `"${String(v).replace(/"/g, '\\"')}"`;
        lines.push(`${k}: ${val}`);
      }
      return lines.join("\n");
    },

    // Helper: YAML lines → object
    _yamlToMap(text) {
      if (!text || !text.trim()) return {};
      const result = {};
      for (const line of text.split("\n")) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith("#")) continue;
        const colonIdx = trimmed.indexOf(":");
        if (colonIdx < 1) continue;
        const key = trimmed.slice(0, colonIdx).trim();
        let val = trimmed.slice(colonIdx + 1).trim();
        // Strip surrounding quotes
        if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) {
          val = val.slice(1, -1);
        }
        result[key] = val;
      }
      return result;
    },

    // Helper: access_modes array → YAML list. Includes access_mode and
    // attachment_mode so the full CSI capability pair is visible and editable.
    _capabilitiesToYaml(modes) {
      if (!Array.isArray(modes) || modes.length === 0) return "";
      const lines = [];
      for (const m of modes) {
        const pairs = [];
        for (const [k, v] of Object.entries(m)) {
          if (v === undefined || v === null || v === "") continue;
          pairs.push(`${k}: "${v}"`);
        }
        if (pairs.length === 0) continue;
        lines.push("- " + pairs[0]);
        for (let i = 1; i < pairs.length; i++) lines.push("  " + pairs[i]);
      }
      return lines.join("\n");
    },

    // Helper: YAML list → access_modes array. access_mode and attachment_mode ARE
    // parsed from the textarea so the full CSI capability pair round-trips; when
    // absent, they fall back to the existing value (set by the volume-type
    // dropdown) or the CSI defaults.
    _yamlToCapabilities(text, existingModes) {
      const parsed = [];
      let current = null;
      for (const line of text.split("\n")) {
        const trimmed = line.trim();
        if (!trimmed) continue;
        if (trimmed.startsWith("- ")) {
          if (current) parsed.push(current);
          current = {};
          const p = this._parseInlineKV(trimmed.slice(2));
          if (p) current[p.key] = p.value;
        } else if (current && trimmed.includes(":")) {
          const p = this._parseInlineKV(trimmed);
          if (p) current[p.key] = p.value;
        }
      }
      if (current) parsed.push(current);

      if (parsed.length === 0) return existingModes || [];

      const result = [];
      for (let i = 0; i < parsed.length; i++) {
        const merged = {};
        const ex = existingModes && existingModes[i];
        merged.access_mode = parsed[i].access_mode || (ex && ex.access_mode) || "single-node-writer";
        merged.attachment_mode = parsed[i].attachment_mode || (ex && ex.attachment_mode) || "file-system";
        for (const [k, v] of Object.entries(parsed[i])) {
          if (k !== "access_mode" && k !== "attachment_mode") merged[k] = v;
        }
        result.push(merged);
      }
      return result;
    },

    // Helper: mount_options (fs_type + mount_flags) → YAML
    _mountOptionsToYaml(fsType, mountFlags) {
      const lines = [];
      if (fsType) lines.push(`fs_type: "${fsType}"`);
      if (Array.isArray(mountFlags) && mountFlags.length > 0) {
        lines.push("mount_flags:");
        for (const f of mountFlags) lines.push(`  - ${f}`);
      }
      return lines.join("\n");
    },

    // Helper: YAML → mount_options object
    _yamlToMountOptions(text) {
      if (!text || !text.trim()) return {};
      const result = { fs_type: "", mount_flags: [] };
      let inFlags = false;
      for (const line of text.split("\n")) {
        const trimmed = line.trim();
        if (trimmed.startsWith("fs_type:")) {
          let val = trimmed.slice(8).trim();
          if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) val = val.slice(1, -1);
          result.fs_type = val;
          inFlags = false;
        } else if (trimmed === "mount_flags:" || trimmed.startsWith("mount_flags:")) {
          inFlags = true;
        } else if (inFlags && trimmed.startsWith("- ")) {
          result.mount_flags.push(trimmed.slice(2).trim());
        } else {
          inFlags = false;
        }
      }
      return result;
    },

    _parseInlineKV(s) {
      const idx = s.indexOf(":");
      if (idx < 1) return null;
      const key = s.slice(0, idx).trim();
      let val = s.slice(idx + 1).trim();
      if ((val.startsWith('"') && val.endsWith('"')) || (val.startsWith("'") && val.endsWith("'"))) val = val.slice(1, -1);
      return { key, value: val };
    },
    wizardAddDevice() {
      this.specWizard.spec.devices.push({ host_path: "", container_path: "", cgroup_permissions: "" });
    },
    wizardRemoveDevice(i) {
      this.specWizard.spec.devices.splice(i, 1);
    },

    async applySpecWizard() {
      if (!this.specWizard.wizardable) {
        this.specWizard.show = false;
        return;
      }
      this.specWizard.saving = true;
      this.specWizard.error = "";
      try {
        const platform = this.formData.platform;
        const originalJob = this.jobEditor ? this.jobEditor.getValue() : this.formData.job;
        const originalVolumes = this.volumeEditor ? this.volumeEditor.getValue() : this.formData.volumes;
        const resp = await fetch("/api/spec/build", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            platform,
            original_job: originalJob,
            original_volumes: originalVolumes,
            spec: this.specWizard.spec,
          }),
        });
        if (!resp.ok) {
          const txt = await resp.text();
          throw new Error(`HTTP ${resp.status}: ${txt}`);
        }
        const data = await resp.json();
        if (this.jobEditor) {
          this.jobEditor.session.setValue(data.job || "");
        } else {
          this.formData.job = data.job || "";
        }
        if (this.volumeEditor) {
          this.volumeEditor.session.setValue(data.volumes || "");
        } else {
          this.formData.volumes = data.volumes || "";
        }
        // Icon write-through: only when wizard.icon is set (picker was used).
        if (this.specWizard.icon) {
          this.formData.icon_url = this.specWizard.icon;
          this.$nextTick(() => this.$dispatch("refresh-autocompleter"));
        }
        this.specWizard.show = false;
        this.$dispatch('show-alert', { msg: "Spec updated via wizard: " + this.generateWizardSummary(this.specWizard.spec), type: 'success' });
        this.validateSpecs();
      } catch (err) {
        this.specWizard.error = "Failed to apply spec: " + err.message;
      } finally {
        this.specWizard.saving = false;
      }
    },

    cancelSpecWizard() {
      this.specWizard.show = false;
    },

    confirmCloseTemplate() {
      if (this.specWizard.show || this.specWizard.volumeDetails.show || this.specWizard.templateEditor.show) {
        return;
      }
      if (this._formDirty) {
        this.discardConfirm.show = true;
      } else {
        this.$dispatch("close-template-form");
      }
    },

    discardChanges() {
      this._formDirty = false;
      this.discardConfirm.show = false;
      this.$dispatch("close-template-form");
    },

    generateWizardSummary(spec) {
      const parts = [];
      if (spec.image) {
        const img = spec.image.replace(/\$\{\{[^}]*\}\}/g, "…");
        parts.push(img.length > 45 ? img.slice(0, 42) + "…" : img);
      }
      if (spec.memory) parts.push(spec.memory);
      if (spec.cpus) {
        parts.push(spec.cpus + (spec.cpu_type === "cores" ? " cores" : (this.formData.platform === "nomad" ? " MHz" : " CPU")));
      }
      if (spec.ports && spec.ports.length) parts.push(spec.ports.length + (spec.ports.length > 1 ? " ports" : " port"));
      if (spec.environment && spec.environment.length) parts.push(spec.environment.length + " env");
      if (spec.storage && spec.storage.length) parts.push(spec.storage.length + (spec.storage.length > 1 ? " volumes" : " volume"));
      if (spec.templates && spec.templates.length) parts.push(spec.templates.length + (spec.templates.length > 1 ? " templates" : " template"));
      if (spec.cap_add && spec.cap_add.length) parts.push(spec.cap_add.length + (spec.cap_add.length > 1 ? " caps" : " cap"));
      return parts.join(" · ");
    },
  };
};
