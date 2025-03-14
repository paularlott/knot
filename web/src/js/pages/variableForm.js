window.variableForm = function(isEdit, templateVarId) {
  return {
    formData: {
      name: "",
      location: "",
      local: false,
      restricted: false,
      value: "",
      protected: false,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Variable',
    isEdit: isEdit,
    stayOnPage: true,
    nameValid: true,
    valueValid: true,
    locationValid: true,

    async initData() {
      focusElement('input[name="name"]');

      if(isEdit) {
        const response = await fetch('/api/templatevars/' + templateVarId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (response.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const v = await response.json();

          this.formData.name = v.name;
          this.formData.location = v.location;
          this.formData.local = v.local;
          this.formData.restricted = v.restricted;
          this.formData.value = v.value;
          this.formData.protected = v.protected;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('_x_darkMode'));
      if(darkMode == null)
        darkMode = true;

      // Create the job editor
      let editor = ace.edit('value');
      editor.session.setValue(this.formData.value);
      editor.session.on('change', () => {
          this.formData.value = editor.getValue();
      });
      editor.setTheme(darkMode ? "ace/theme/github_dark" : "ace/theme/github");
      editor.session.setMode("ace/mode/text");
      editor.setOptions({
        printMargin: false,
        newLineMode: 'unix',
        tabSize: 2,
        wrap: false,
        vScrollBarAlwaysVisible: true,
        customScrollbar: true,
      });

      // Listen for the theme_change event on the body & change the editor theme
      window.addEventListener('theme-change', function (e) {
        if (e.detail.dark_theme) {
          editor.setTheme("ace/theme/github_dark");
        } else {
          editor.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    checkName() {
      return this.nameValid = validate.varName(this.formData.name);
    },
    checkValue() {
      return this.valueValid = validate.maxLength(this.formData.value, 10 * 1024 * 1024);
    },
    checkLocation() {
      return this.locationValid = this.formData.local || validate.maxLength(this.formData.location, 64);
    },

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkValue() || err;
      err = !this.checkLocation() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating variable...' : 'Create variable...'
      }
      this.loading = true;

      fetch(isEdit ? '/api/templatevars/' + templateVarId : '/api/templatevars', {
          method: isEdit ? 'PUT' : 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            if(self.stayOnPage) {
              self.$dispatch('show-alert', { msg: "Variable updated", type: 'success' });
            } else {
              window.location.href = '/variables';
            }
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Variable created", type: 'success' });
            window.location.href = '/variables';
          } else {
            response.json().then((data) => {
              self.$dispatch('show-alert', { msg: "Failed to update the variable, " + data.error, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: 'Ooops Error!<br />' + error.message, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Variable';
          this.loading = false;
        })
    },
  }
}
