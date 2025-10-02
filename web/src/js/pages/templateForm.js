import { validate } from '../validators.js';
import { focus } from '../focus.js';

/// wysiwyg editor
import ace from 'ace-builds/src-noconflict/ace';
import 'ace-builds/src-noconflict/mode-terraform';
import 'ace-builds/src-noconflict/mode-yaml';
import 'ace-builds/src-noconflict/mode-text';
import 'ace-builds/src-noconflict/theme-github';
import 'ace-builds/src-noconflict/theme-github_dark';
import 'ace-builds/src-noconflict/ext-searchbox';

window.templateForm = function(isEdit, templateId) {
  focus.Element('input[name="name"]');

  return {
    formData: {
      name: "",
      description: "",
      job: "",
      volumes: "",
      groups: [],
      zones: [],
      custom_fields: [],
      platform: "nomad",
      with_terminal: false,
      with_vscode_tunnel: false,
      with_code_server: false,
      with_ssh: false,
      with_run_command: false,
      compute_units: 0,
      storage_units: 0,
      active: true,
      max_uptime: 0,
      max_uptime_unit: 'disabled',
      schedule_enabled: false,
      auto_start: false,
      icon_url: '',
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
    isEdit,
    stayOnPage: true,
    buttonLabel: isEdit ? 'Update' : 'Create Template',
    nameValid: true,
    jobValid: true,
    volValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
    uptimeValid: true,
    groups: [],
    fromHours: [],
    toHours: [],
    zoneValid: [],
    customFieldValid: [],
    showPlatformWarning: false,

    async initData() {
      for (let hour = 0; hour < 24; hour++) {
        for (let minute = 0; minute < 60; minute += 15) {
          const period = hour < 12 || hour === 24 ? 'am' : 'pm';
          const displayHour = hour % 12 === 0 ? 12 : hour % 12;
          const displayMinute = minute === 0 ? '00' : minute;
          this.fromHours.push(`${displayHour}:${displayMinute}${period}`);
          this.toHours.push(`${displayHour}:${displayMinute}${period}`);
        }
      }
      this.toHours.push('11:59pm');

      const groupsResponse = await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      const groupsList = await groupsResponse.json();
      this.groups = groupsList.groups;

      if(isEdit) {
        const templateResponse = await fetch(`/api/templates/${templateId}`, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (templateResponse.status !== 200) {
          window.location.href = '/spaces';
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
        }

        // If this is edit and duplicate then change to add
        if (window.location.hash === '#duplicate') {
          this.formData.name = `Copy of ${this.formData.name}`;
          this.isEdit = isEdit = false;
          this.buttonLabel = 'Create Template';
        }
      }

      let darkMode = this.darkMode;
      if(darkMode == null)
        darkMode = true;

      // Create the job editor
      const editor = ace.edit('job');
      editor.session.setValue(this.formData.job);
      editor.session.on('change', () => {
        this.formData.job = editor.getValue();
      });
      editor.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editor.session.setMode("ace/mode/terraform");
      editor.setOptions({
        printMargin: false,
        newLineMode: 'unix',
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
      });

      // Create the volume editor
      const editorVol = ace.edit('vol');
      editorVol.session.setValue(this.formData.volumes);
      editorVol.session.on('change', () => {
        this.formData.volumes = editorVol.getValue();
      });
      editorVol.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editorVol.session.setMode("ace/mode/yaml");
      editorVol.setOptions({
        printMargin: false,
        newLineMode: 'unix',
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      // Create the description editor
      const editorDesc = ace.edit('description');
      editorDesc.session.setValue(this.formData.description);
      editorDesc.session.on('change', () => {
        this.formData.description = editorDesc.getValue();
      });
      editorDesc.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editorDesc.session.setMode("ace/mode/text");
      editorDesc.setOptions({
        printMargin: false,
        newLineMode: 'unix',
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
        useWorker: false,
      });

      // Listen for the theme_change event on the body & change the editor theme
      window.addEventListener('theme-change', (e) => {
        if (e.detail.dark_theme) {
          editor.setTheme("ace/theme/github_dark");
          editorVol.setTheme("ace/theme/github_dark");
          editorDesc.setTheme("ace/theme/github_dark");
        } else {
          editor.setTheme("ace/theme/github");
          editorVol.setTheme("ace/theme/github");
          editorDesc.setTheme("ace/theme/github");
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
      this.formData.schedule[day].enabled = !this.formData.schedule[day].enabled;
    },
    checkPlatform() {
      return validate.isOneOf(this.formData.platform, ["manual", "docker", "podman", "nomad", "apple"])
    },
    checkName() {
      this.nameValid = validate.templateName(this.formData.name);
      return this.nameValid;
    },
    checkJob() {
      this.jobValid = this.formData.platform === 'manual' || validate.required(this.formData.job);
      return this.jobValid;
    },
    checkComputeUnits() {
      this.computeUnitsValid = validate.isNumber(this.formData.compute_units, 0, Infinity);
      return this.computeUnitsValid;
    },
    checkStorageUnits() {
      this.storageUnitsValid = validate.isNumber(this.formData.storage_units, 0, Infinity);
      return this.storageUnitsValid;
    },
    checkUptime() {
      if(this.formData.max_uptime_unit === 'disabled') {
        this.uptimeValid = true;
      } else {
        this.uptimeValid = validate.isNumber(this.formData.max_uptime, 0, Infinity) && validate.isOneOf(this.formData.max_uptime_unit, ['minute', 'hour', 'day']);
      }
      return this.uptimeValid;
    },
    checkZonesValid() {
      let zonesValid = true
      this.formData.zones.forEach((zone, index) => {
        zonesValid = zonesValid && this.zoneValid[index];
      });
      return zonesValid;
    },
    checkCustomFieldsValid() {
      let fieldsValid = true
      this.formData.custom_fields.forEach((field, index) => {
        fieldsValid = fieldsValid && this.customFieldValid[index];
      });
      return fieldsValid;
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkJob() || err;
      err = !this.checkPlatform() || err;
      err = !this.checkZonesValid() || err;
      err = !this.checkCustomFieldsValid() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = this.isEdit ? 'Updating template...' : 'Create template...'
      }
      this.loading = true;

      const data = {
        name: this.formData.name,
        description: this.formData.description,
        job: this.formData.platform === 'manual' ? "" : this.formData.job,
        volumes: this.formData.platform === 'manual' ? "" : this.formData.volumes,
        groups: this.formData.groups,
        with_terminal: this.formData.with_terminal,
        with_vscode_tunnel: this.formData.with_vscode_tunnel,
        with_code_server: this.formData.with_code_server,
        with_ssh: this.formData.with_ssh,
        with_run_command: this.formData.with_run_command,
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
        schedule_enabled: this.formData.schedule_enabled && this.formData.platform !== 'manual',
        auto_start: this.formData.auto_start,
        schedule: this.formData.schedule,
        zones: this.formData.zones,
        active: this.formData.active,
        max_uptime: parseInt(this.formData.max_uptime),
        max_uptime_unit: this.formData.platform !== 'manual' ? 'disabled' : this.formData.max_uptime_unit,
        platform: this.formData.platform,
        icon_url: this.formData.icon_url,
        custom_fields: this.formData.custom_fields,
      };

      await fetch(isEdit ? `/api/templates/${templateId}` : '/api/templates', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 200) {

            if(!this.stayOnPage) {
              window.location.href = '/templates';
              return;
            }

            self.$dispatch('show-alert', { msg: "Template updated", type: 'success' });
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Template created", type: 'success' });
            window.location.href = '/templates';
          } else {
            response.json().then((d) => {
              self.$dispatch('show-alert', { msg: `Failed to update the template, ${d.error}`, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = this.isEdit ? 'Update' : 'Create Template';
          this.loading = false;
        })
    },
    getDayOfWeek(day) {
      return ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'][day];
    },
    addZone() {
      this.zoneValid.push(true);
      this.formData.zones.push('');
    },
    removeZone(index) {
      this.formData.zones.splice(index, 1);
      this.zoneValid.splice(index, 1);
    },
    checkZone(index) {
      if(index >= 0 && index < this.formData.zones.length) {
        let isValid = validate.maxLength(this.formData.zones[index], 64);

        // If valid then check for duplicate extra name
        if(isValid) {
          for (let i = 0; i < this.formData.zones.length; i++) {
            if(i !== index && this.formData.zones[i] === this.formData.zones[index]) {
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
      this.formData.custom_fields.push({name: '', description: ''});
    },
    removeField(index) {
      this.formData.custom_fields.splice(index, 1);
      this.customFieldValid.splice(index, 1);
    },
    checkCustomField(index) {
      if(index >= 0 && index < this.formData.custom_fields.length) {
        let isValid = validate.maxLength(this.formData.custom_fields[index].name, 24) &&
                      validate.varName(this.formData.custom_fields[index].name) &&
                      validate.maxLength(this.formData.custom_fields[index].description, 256);

        // If valid then check for duplicate name
        if(isValid) {
          for (let i = 0; i < this.formData.custom_fields.length; i++) {
            if(i !== index && this.formData.custom_fields[i].name === this.formData.custom_fields[index].name) {
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
    isLocalContainer() {
      return this.formData.platform === 'docker' || this.formData.platform === 'podman' || this.formData.platform === 'apple';
    }
  }
}
