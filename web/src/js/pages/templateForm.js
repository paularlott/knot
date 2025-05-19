window.templateForm = function(isEdit, templateId) {
  focusElement('input[name="name"]');

  return {
    formData: {
      name: "",
      description: "",
      job: "",
      volumes: "",
      groups: [],
      locations: [],
      local_container: false,
      is_manual: false,
      with_terminal: false,
      with_vscode_tunnel: false,
      with_code_server: false,
      with_ssh: false,
      compute_units: 0,
      storage_units: 0,
      active: true,
      schedule_enabled: false,
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
    stayOnPage: true,
    buttonLabel: isEdit ? 'Update' : 'Create Template',
    nameValid: true,
    jobValid: true,
    volValid: true,
    computeUnitsValid: true,
    storageUnitsValid: true,
    groups: [],
    fromHours: [],
    toHours: [],
    locationValid: [],

    async initData() {

      for (let hour = 0; hour < 24; hour++) {
        for (let minute = 0; minute < 60; minute += 15) {
          let period = hour < 12 || hour === 24 ? 'am' : 'pm';
          let displayHour = hour % 12 === 0 ? 12 : hour % 12;
          let displayMinute = minute === 0 ? '00' : minute;
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
      groupsList = await groupsResponse.json();
      this.groups = groupsList.groups;

      if(isEdit) {
        const templateResponse = await fetch('/api/templates/' + templateId, {
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
          this.formData.local_container = template.local_container;
          this.formData.is_manual = template.is_manual;
          this.formData.with_terminal = template.with_terminal;
          this.formData.with_vscode_tunnel = template.with_vscode_tunnel;
          this.formData.with_code_server = template.with_code_server;
          this.formData.with_ssh = template.with_ssh;
          this.formData.compute_units = template.compute_units;
          this.formData.storage_units = template.storage_units;
          this.formData.active = template.active;
          this.formData.schedule_enabled = template.schedule_enabled;
          this.formData.schedule = template.schedule;

          // Set the locations and mark all as valid
          this.formData.locations =template.locations ? template.locations : [];
          this.locationValid = [];
          this.formData.locations.forEach(() => {
            this.locationValid.push(true);
          });
        }
      }

      let darkMode = this.darkMode;
      if(darkMode == null)
        darkMode = true;

      // Create the job editor
      let editor = ace.edit('job');
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
      let editorVol = ace.edit('vol');
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
      let editorDesc = ace.edit('description');
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
      window.addEventListener('theme-change', function (e) {
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
    toggleLocalContainer() {
      this.formData.local_container = !this.formData.local_container;
      if(this.formData.local_container) {
        this.formData.is_manual = false;
      }
    },
    toggleIsManual() {
      this.formData.is_manual = !this.formData.is_manual;
      if(this.formData.is_manual) {
        this.formData.local_container = false;
      }
    },
    toggleActive() {
      this.formData.active = !this.formData.active;
    },
    toggleWithTerminal() {
      this.formData.with_terminal = !this.formData.with_terminal;
    },
    toggleWithVSCodeTunnel() {
      this.formData.with_vscode_tunnel = !this.formData.with_vscode_tunnel;
    },
    toggleWithCodeServer() {
      this.formData.with_code_server = !this.formData.with_code_server;
    },
    toggleWithSSH() {
      this.formData.with_ssh = !this.formData.with_ssh;
    },
    toggleSchduleEnabled() {
      this.formData.schedule_enabled = !this.formData.schedule_enabled;
    },
    toggleDaySchedule(day) {
      this.formData.schedule[day].enabled = !this.formData.schedule[day].enabled;
    },
    checkName() {
      return this.nameValid = validate.name(this.formData.name);
    },
    checkJob() {
      return this.jobValid = this.formData.is_manual || validate.required(this.formData.job);
    },
    checkComputeUnits() {
      return this.computeUnitsValid = validate.isNumber(this.formData.compute_units, 0, Infinity);
    },
    checkStorageUnits() {
      return this.storageUnitsValid = validate.isNumber(this.formData.storage_units, 0, Infinity);
    },

    async submitData() {
      let err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkJob() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating template...' : 'Create template...'
      }
      this.loading = true;

      let data = {
        name: this.formData.name,
        description: this.formData.description,
        job: this.formData.is_manual ? "" : this.formData.job,
        volumes: this.formData.is_manual ? "" : this.formData.volumes,
        groups: this.formData.groups,
        with_terminal: this.formData.with_terminal,
        with_vscode_tunnel: this.formData.with_vscode_tunnel,
        with_code_server: this.formData.with_code_server,
        with_ssh: this.formData.with_ssh,
        compute_units: parseInt(this.formData.compute_units),
        storage_units: parseInt(this.formData.storage_units),
        schedule_enabled: this.formData.schedule_enabled,
        schedule: this.formData.schedule,
        locations: this.formData.locations,
        active: this.formData.active,
      };

      if(!isEdit) {
        data.local_container = this.formData.local_container;
        data.is_manual = this.formData.is_manual;
      }

      fetch(isEdit ? '/api/templates/' + templateId : '/api/templates', {
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
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: "Failed to update the template, " + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Template';
          this.loading = false;
        })
    },
    getDayOfWeek(day) {
      return ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'][day];
    },
    async addLocation() {
      this.locationValid.push(true);
      this.formData.locations.push('');
    },
    async removeLocation(index) {
      this.formData.locations.splice(index, 1);
      this.locationValid.splice(index, 1);
    },
    checkLocation(index) {
      if(index >= 0 && index < this.formData.locations.length) {
        let isValid = validate.maxLength(this.formData.locations[index], 64);

        // If valid then check for duplicate extra name
        if(isValid) {
          for (let i = 0; i < this.formData.locations.length; i++) {
            if(i !== index && this.formData.locations[i] === this.formData.locations[index]) {
              isValid = false;
              break;
            }
          }
        }

        return this.locationValid[index] = isValid;
      } else {
        return false;
      }
    },
  }
}
