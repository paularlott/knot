window.volumeForm = function(isEdit, volumeId) {
  return {
    formData: {
      name: "",
      definition: "",
      local_container: false,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Volume',
    nameValid: true,
    volValid: true,
    isEdit: isEdit,
    stayOnPage: true,

    async initData() {
      focusElement('input[name="name"]');

      if(isEdit) {
        const volumeResponse = await fetch('/api/volumes/' + volumeId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (volumeResponse.status !== 200) {
          window.location.href = '/volumes';
        } else {
          const volume = await volumeResponse.json();

          this.formData.name = volume.name;
          this.formData.definition = volume.definition;
          this.formData.local_container = volume.local_container;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('_x_darkMode'));
      if(darkMode == null)
        darkMode = true;

      // Create the volume editor
      let editorVol = ace.edit('vol');
      editorVol.session.setValue(this.formData.definition);
      editorVol.session.on('change', () => {
          this.formData.definition = editorVol.getValue();
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
          editorVol.setTheme("ace/theme/github_dark");
        } else {
          editorVol.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.name(this.formData.name);
    },
    checkVol() {
      return this.volValid = validate.required(this.formData.definition);
    },
    toggleLocalContainer() {
      this.formData.local_container = !this.formData.local_container;
    },

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkVol() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating volume...' : 'Create volume...'
      }
      this.loading = true;

      var data = {
        name: this.formData.name,
        definition: this.formData.definition,
      }

      if(!isEdit) {
        data.local_container = this.formData.local_container;
      }

      fetch(isEdit ? '/api/volumes/' + volumeId : '/api/volumes', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(data)
        })
        .then((response) => {
          if (response.status === 200) {
            if(self.stayOnPage) {
              self.$dispatch('show-alert', { msg: "Volume updated", type: 'success' });
            } else {
              window.location.href = '/volumes';
            }
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Volume created", type: 'success' });
            window.location.href = '/volumes';
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: "Failed to update the volume, " + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Volume';
          this.loading = false;
        })
    },
  }
}
