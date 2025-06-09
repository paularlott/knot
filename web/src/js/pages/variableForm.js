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

window.variableForm = function(isEdit, templateVarId, isLeafServer) {
  return {
    formData: {
      name: "",
      zone: "",
      local: isLeafServer ? true : false,
      restricted: isLeafServer ? false : true,
      value: "",
      protected: false,
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Variable',
    isEdit,
    stayOnPage: true,
    nameValid: true,
    valueValid: true,
    zoneValid: true,

    async initData() {
      focus.Element('input[name="name"]');

      if(isEdit) {
        const response = await fetch(`/api/templatevars/${templateVarId}`, {
          headers: {
            'Content-Type': 'application/json'
          }
        });

        if (response.status !== 200) {
          window.location.href = '/spaces';
        } else {
          const varData = await response.json();

          this.formData.name = varData.name;
          this.formData.zone = varData.zone;
          this.formData.local = varData.local;
          this.formData.restricted = varData.restricted;
          this.formData.value = varData.value;
          this.formData.protected = varData.protected;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('_x_darkMode'));
      if(darkMode == null)
        darkMode = true;

      // Create the job editor
      const editor = ace.edit('value');
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
      window.addEventListener('theme-change', (e) => {
        if (e.detail.dark_theme) {
          editor.setTheme("ace/theme/github_dark");
        } else {
          editor.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    checkName() {
      this.nameValid = validate.varName(this.formData.name);
      return this.nameValid;
    },
    checkValue() {
      this.valueValid = validate.maxLength(this.formData.value, 10 * 1024 * 1024);
      return this.valueValid;
    },
    checkZone() {
      this.zoneValid = this.formData.local || validate.maxLength(this.formData.zone, 64);
      return this.zoneValid;
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkValue() || err;
      err = !this.checkZone() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating variable...' : 'Create variable...'
      }
      this.loading = true;

      await fetch(isEdit ? `/api/templatevars/${templateVarId}` : '/api/templatevars', {
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
            response.json().then((d) => {
              self.$dispatch('show-alert', { msg: `Failed to update the variable, ${d.error}`, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Variable';
          this.loading = false;
        })
    },
  }
}
