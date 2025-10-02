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

window.volumeForm = function(isEdit, volumeId) {
  return {
    formData: {
      name: "",
      definition: "",
      platform: 'nomad',
    },
    loading: true,
    buttonLabel: isEdit ? 'Update' : 'Create Volume',
    nameValid: true,
    volValid: true,
    isEdit,
    stayOnPage: true,
    showPlatformWarning: false,

    async initData() {
      focus.Element('input[name="name"]');

      if(isEdit) {
        const volumeResponse = await fetch(`/api/volumes/${volumeId}`, {
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
          this.formData.platform = volume.platform;
        }
      }

      let darkMode = JSON.parse(localStorage.getItem('_x_darkMode'));
      if(darkMode == null)
        darkMode = true;

      // Create the volume editor
      const editorVol = ace.edit('vol');
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
      window.addEventListener('theme-change', (e) => {
        if (e.detail.dark_theme) {
          editorVol.setTheme("ace/theme/github_dark");
        } else {
          editorVol.setTheme("ace/theme/github");
        }
      });

      this.loading = false;
    },
    checkName() {
      this.nameValid = validate.name(this.formData.name);
      return this.nameValid;
    },
    checkVol() {
      this.volValid = validate.required(this.formData.definition);
      return this.volValid;
    },
    checkPlatform() {
      return validate.isOneOf(this.formData.platform, ["docker", "podman", "nomad", "apple"])
    },

    async submitData() {
      let err = false;
      const self = this;
      err = !this.checkName() || err;
      err = !this.checkVol() || err;
      err = !this.checkPlatform() || err;
      if(err) {
        return;
      }

      if(this.stayOnPage) {
        this.buttonLabel = isEdit ? 'Updating volume...' : 'Create volume...'
      }
      this.loading = true;

      const data = {
        name: this.formData.name,
        definition: this.formData.definition,
        platform: this.formData.platform,
      }

      await fetch(isEdit ? `/api/volumes/${volumeId}` : '/api/volumes', {
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
            response.json().then((d) => {
              self.$dispatch('show-alert', { msg: `Failed to update the volume, ${d.error}`, type: 'error' });
            });
          }
        })
        .catch((error) => {
          self.$dispatch('show-alert', { msg: `Error!<br />${error.message}`, type: 'error' });
        })
        .finally(() => {
          this.buttonLabel = isEdit ? 'Update' : 'Create Volume';
          this.loading = false;
        })
    },
  }
}
