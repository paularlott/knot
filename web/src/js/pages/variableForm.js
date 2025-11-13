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
      zones: [],
      local: isLeafServer ? true : false,
      restricted: isLeafServer ? false : true,
      value: "",
      protected: false,
    },
    loading: true,
    buttonLabel: isEdit ? 'Save Changes' : 'Create Variable',
    isEdit,
    stayOnPage: true,
    nameValid: true,
    valueValid: true,
    zoneValid: [],

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
          this.formData.zones = varData.zones;
          this.formData.local = varData.local;
          this.formData.restricted = varData.restricted;
          this.formData.value = varData.value;
          this.formData.protected = varData.protected;

          this.zoneValid = [];
          this.formData.zones.forEach(() => {
            this.zoneValid.push(true);
          });
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
    checkZonesValid() {
      let zonesValid = true
      this.formData.zones.forEach((zone, index) => {
        zonesValid = zonesValid && this.zoneValid[index];
      });
      return zonesValid;
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
    addZone() {
      this.zoneValid.push(true);
      this.formData.zones.push('');
    },
    removeZone(index) {
      this.formData.zones.splice(index, 1);
      this.zoneValid.splice(index, 1);
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkValue() || err;
      err = !this.checkZonesValid() || err;
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
            self.$dispatch('show-alert', { msg: "Variable updated", type: 'success' });
            self.$dispatch('close-variable-form');
          } else if (response.status === 201) {
            self.$dispatch('show-alert', { msg: "Variable created", type: 'success' });
            self.$dispatch('close-variable-form');
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
