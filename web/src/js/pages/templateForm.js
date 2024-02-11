window.templateForm = function(isEdit, templateId) {
  focusElement('input[name="name"]');

  return {
    formData: {
      name: "",
      job: "",
      volumes: "",
      groups: [],
    },
    loading: true,
    buttonLabel: isEdit ? 'Update Template' : 'Create Template',
    nameValid: true,
    jobValid: true,
    volValid: true,
    groups: [],

    async initData() {
      const groupsResponse = await fetch('/api/v1/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      });
      this.groups = await groupsResponse.json();

      if(isEdit) {
        const templateResponse = await fetch('/api/v1/templates/' + templateId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (templateResponse.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const template = await templateResponse.json();

          this.formData.name = template.name;
          this.formData.job = template.job;
          this.formData.volumes = template.volumes;
          this.formData.groups = template.groups;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('darkMode'));
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

      // Listen for the theme_change event on the body & change the editor theme
      window.addEventListener('theme-change', function (e) {
        if (e.detail.dark_theme) {
          editor.setTheme("ace/theme/github_dark");
          editorVol.setTheme("ace/theme/github_dark");
        } else {
          editor.setTheme("ace/theme/github");
          editorVol.setTheme("ace/theme/github");
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
    checkName() {
      return this.nameValid = validate.name(this.formData.name);
    },
    checkJob() {
      return this.jobValid = validate.required(this.formData.job);
    },

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkJob() || err;
      if(err) {
        return;
      }

      this.buttonLabel = isEdit ? 'Updating template...' : 'Create template...'
      this.loading = true;

      fetch(isEdit ? '/api/v1/templates/' + templateId : '/api/v1/templates', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Template updated", type: 'success' });
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Template created", type: 'success' });
            response.json().then(function(data) {
              window.location.href = '/templates/edit/' + data.template_id;
            });
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
          this.buttonLabel = isEdit ? 'Update Template' : 'Create Template';
          this.loading = false;
        })
    },
  }
}
