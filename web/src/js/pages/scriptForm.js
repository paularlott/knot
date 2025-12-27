/// wysiwyg editor
import ace from 'ace-builds/src-noconflict/ace';
import 'ace-builds/src-noconflict/mode-python';
import 'ace-builds/src-noconflict/mode-toml';
import 'ace-builds/src-noconflict/mode-text';
import 'ace-builds/src-noconflict/theme-github';
import 'ace-builds/src-noconflict/theme-github_dark';
import 'ace-builds/src-noconflict/ext-searchbox';
import { focus } from '../focus.js';

window.scriptForm = function(isEdit, scriptId) {
  return {
    loading: true,
    isEdit: isEdit,
    scriptId: scriptId,
    nameValid: true,
    contentValid: true,
    buttonLabel: isEdit ? 'Update Script' : 'Create Script',
    formData: {
      name: '',
      description: '',
      content: '',
      groups: [],
      active: true,
      script_type: 'script',
      mcp_input_schema_toml: '',
      mcp_keywords: [],
      timeout: '',
    },
    mcpKeywordsStr: '',
    availableGroups: [],
    contentEditor: null,
    schemaEditor: null,
    darkMode: Alpine.$persist(null).as('dark-theme').using(localStorage),

    async initData() {
      focus.Element('input[name="name"]');

      await this.loadGroups();

      if (this.isEdit) {
        await fetch(`/api/scripts/${this.scriptId}`, {
          headers: {
            'Content-Type': 'application/json'
          }
        }).then((response) => {
          if (response.status === 200) {
            response.json().then((data) => {
              this.formData = {
                name: data.name,
                description: data.description,
                content: data.content,
                groups: data.groups || [],
                active: data.active,
                script_type: data.script_type || 'script',
                mcp_input_schema_toml: data.mcp_input_schema_toml || '',
                mcp_keywords: data.mcp_keywords || [],
                timeout: data.timeout || '',
              };
              this.mcpKeywordsStr = this.formData.mcp_keywords.join(', ');
              this.initEditors();
              this.loading = false;
            });
          } else if (response.status === 401) {
            window.location.href = '/logout';
          }
        }).catch(() => {});
      } else {
        this.initEditors();
        this.loading = false;
      }
    },

    initEditors() {
      this.$nextTick(() => {
        let darkMode = this.darkMode;
        if (darkMode == null) darkMode = true;

        if (!this.contentEditor) {
          this.contentEditor = ace.edit('content');
          this.contentEditor.session.setValue(this.formData.content);
          this.contentEditor.session.on('change', () => {
            this.formData.content = this.contentEditor.getValue();
          });
          this.contentEditor.setTheme(darkMode ? 'ace/theme/github_dark' : 'ace/theme/github');
          this.contentEditor.session.setMode('ace/mode/python');
          this.contentEditor.setOptions({
            printMargin: false,
            newLineMode: 'unix',
            tabSize: 2,
            wrap: false,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
          });
        }

        if (!this.schemaEditor) {
          this.schemaEditor = ace.edit('mcp_schema');
          this.schemaEditor.session.setValue(this.formData.mcp_input_schema_toml);
          this.schemaEditor.session.on('change', () => {
            this.formData.mcp_input_schema_toml = this.schemaEditor.getValue();
          });
          this.schemaEditor.setTheme(darkMode ? 'ace/theme/github_dark' : 'ace/theme/github');
          this.schemaEditor.session.setMode('ace/mode/toml');
          this.schemaEditor.setOptions({
            printMargin: false,
            newLineMode: 'unix',
            tabSize: 2,
            wrap: false,
            vScrollBarAlwaysVisible: true,
            customScrollbar: true,
            useWorker: false,
          });
        }

        window.addEventListener('theme-change', (e) => {
          if (e.detail.dark_theme) {
            this.contentEditor.setTheme('ace/theme/github_dark');
            this.schemaEditor.setTheme('ace/theme/github_dark');
          } else {
            this.contentEditor.setTheme('ace/theme/github');
            this.schemaEditor.setTheme('ace/theme/github');
          }
        });
      });
    },

    async loadGroups() {
      await fetch('/api/groups', {
        headers: {
          'Content-Type': 'application/json'
        }
      }).then((response) => {
        if (response.status === 200) {
          response.json().then((data) => {
            this.availableGroups = data.groups || [];
          });
        }
      }).catch(() => {});
    },

    checkName() {
      this.nameValid = /^[a-zA-Z0-9_]{1,64}$/.test(this.formData.name);
    },

    toggleGroup(groupId) {
      const index = this.formData.groups.indexOf(groupId);
      if (index >= 0) {
        this.formData.groups.splice(index, 1);
      } else {
        this.formData.groups.push(groupId);
      }
    },

    async submitData() {
      this.checkName();
      this.contentValid = this.formData.content.length <= 4 * 1024 * 1024;

      if (!this.nameValid || !this.contentValid) {
        return;
      }

      this.formData.mcp_keywords = this.mcpKeywordsStr.split(',').map(k => k.trim()).filter(k => k.length > 0);

      const submitData = { ...this.formData };
      submitData.timeout = submitData.timeout === '' ? 0 : parseInt(submitData.timeout);

      this.loading = true;
      const url = this.isEdit ? `/api/scripts/${this.scriptId}` : '/api/scripts';
      const method = this.isEdit ? 'PUT' : 'POST';

      await fetch(url, {
        method: method,
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(submitData)
      }).then(async (response) => {
        if (response.status === 200) {
          this.$dispatch('show-alert', { msg: "Script updated", type: 'success' });
          this.$dispatch('close-script-form');
        } else if (response.status === 201) {
          this.$dispatch('show-alert', { msg: "Script created", type: 'success' });
          this.$dispatch('close-script-form');
        } else if (response.status === 401) {
          window.location.href = '/logout';
        } else {
          try {
            const data = await response.json();
            this.$dispatch('show-alert', { msg: data.error || "Failed to save script", type: 'error' });
          } catch (e) {
            const text = await response.text();
            this.$dispatch('show-alert', { msg: text || "Failed to save script", type: 'error' });
          }
        }
        this.loading = false;
      }).catch((err) => {
        this.$dispatch('show-alert', { msg: "Network error: " + err.message, type: 'error' });
        this.loading = false;
      });
    },
  };
}
