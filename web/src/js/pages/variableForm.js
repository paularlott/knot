window.variableForm = function(isEdit, templateVarId) {
  return {
    formData: {
      name: "",
      value: "",
      protected: false,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update Variable' : 'Create Variable',
    nameValid: true,
    valueValid: true,

    async initData() {
      focusElement('input[name="name"]');

      if(isEdit) {
        const response = await fetch('/api/v1/templatevars/' + templateVarId, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (response.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const v = await response.json();

          this.formData.name = v.name;
          this.formData.value = v.value;
          this.formData.protected = v.protected;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('darkMode'));
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

    async submitData() {
      var err = false,
          self = this;
      err = !this.checkName() || err;
      err = !this.checkValue() || err;
      if(err) {
        return;
      }

      this.buttonLabel = isEdit ? 'Updating variable...' : 'Create variable...'
      this.loading = true;

      fetch(isEdit ? '/api/v1/templatevars/' + templateVarId : '/api/v1/templatevars', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json'
          },
          body: JSON.stringify(this.formData)
        })
        .then((response) => {
          if (response.status === 200) {
            self.$dispatch('show-alert', { msg: "Variable updated", type: 'success' });
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Variable created", type: 'success' });
            response.json().then(function(data) {
              window.location.href = '/variables/edit/' + data.templatevar_id;
            });
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
          this.buttonLabel = isEdit ? 'Update Variable' : 'Create Variable';
          this.loading = false;
        })
    },
  }
}
