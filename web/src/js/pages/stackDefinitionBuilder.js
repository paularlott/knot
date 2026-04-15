import Alpine from "alpinejs";

function tomlStr(val) {
  if (val == null) return '""';
  const s = String(val);
  if (/["\\\n\r\t]/.test(s)) return '"' + s.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n').replace(/\r/g, '\\r').replace(/\t/g, '\\t') + '"';
  return '"' + s + '"';
}

const blankComponent = () => ({
  name: '',
  template_id: '',
  template_name: '',
  description: '',
  shell: '',
  startup_script_id: '',
  startup_script_name: '',
  depends_on: [],
  custom_fields: [],
  port_forwards: [],
});

const blankForm = () => ({
  name: '',
  description: '',
  active: true,
  scope: 'global',
  groups: [],
  zones: [],
  spaces: [],
});

/**
 * Returns the editor state and methods to be mixed into the stackListComponent.
 * This is kept in a separate file so the core stackListComponent stays clean
 * and easy to merge with upstream changes.
 */
window.stackDefinitionBuilder = function () {
  return {
    // Component edit modal
    componentEditor: {
      show: false,
      index: null,
      form: blankComponent(),
    },

    // Delete component confirmation
    deleteCompConfirm: {
      show: false,
      index: null,
    },

    // Discard unsaved changes confirmation
    discardConfirm: {
      show: false,
    },

    // Editor state
    editor: {
      active: false,
      defId: null,
      dirty: false,
      saving: false,
      validating: false,
      valid: true,
      errors: [],
      form: blankForm(),
      templates: [],
      scripts: [],
      groups: [],
      zoneValid: [],
    },

    // ---- Helpers ----
    groupName(gid) {
      const g = this.editor.groups.find(gr => gr.group_id === gid);
      return g ? g.name : gid;
    },

    toggleGroup(groupId) {
      if (this.editor.form.groups.includes(groupId)) {
        const index = this.editor.form.groups.indexOf(groupId);
        this.editor.form.groups.splice(index, 1);
      } else {
        this.editor.form.groups.push(groupId);
      }
      this.markDirty();
    },

    markDirty() {
      this.editor.dirty = true;
    },

    async autoValidate() {
      if (this.editor.validating || this.editor.saving || !this.editor.active) return;
      this.editor.validating = true;
      try {
        const payload = this.buildRequestPayload();
        const res = await fetch('/api/stack-definitions/validate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (res.ok) {
          const data = await res.json();
          this.editor.valid = data.valid;
          this.editor.errors = data.valid ? [] : (data.errors || []);
        }
      } catch (e) { /* ignore */ }
      this.editor.validating = false;
    },

    canEditDef(def) {
      if (this.isLeafNode) return false;
      if (def.user_id && def.user_id === this.currentUserId) return true;
      if (!def.user_id) return this.permissionManageStackDefinitions;
      return false;
    },

    // ---- Zones (same pattern as template form) ----
    addZone() {
      this.editor.zoneValid.push(true);
      this.editor.form.zones.push('');
      this.markDirty();
    },

    removeZone(index) {
      this.editor.form.zones.splice(index, 1);
      this.editor.zoneValid.splice(index, 1);
      this.markDirty();
    },

    checkZone(index) {
      if (index >= 0 && index < this.editor.form.zones.length) {
        let isValid = this.editor.form.zones[index].length <= 64;
        if (isValid) {
          for (let i = 0; i < this.editor.form.zones.length; i++) {
            if (i !== index && this.editor.form.zones[i] === this.editor.form.zones[index]) {
              isValid = false;
              break;
            }
          }
        }
        this.editor.zoneValid[index] = isValid;
      }
    },

    // ---- Editor lifecycle ----
    createDefinition(isPersonal) {
      this.editor.active = true;
      this.editor.defId = null;
      this.editor.dirty = false;
      this.editor.valid = true;
      this.editor.errors = [];
      this.editor.form = { ...blankForm(), scope: isPersonal ? 'personal' : 'global' };
      this.editor.zoneValid = [];
      this.loadReferenceData();

      if (isPersonal) {
        this.showMyDefs = true;
      } else {
        this.showGlobalDefs = true;
      }
    },

    editDefinition(defId) {
      const def = this.definitions.find(d => d.stack_definition_id === defId);
      if (!def) return;

      this.editor.defId = def.stack_definition_id;
      this.editor.dirty = false;
      this.editor.valid = true;
      this.editor.errors = [];

      const spaces = (def.spaces || []).map(s => ({
        ...s,
        template_name: '',
        startup_script_name: '',
      }));

      this.editor.form = {
        name: def.name || '',
        description: def.description || '',
        active: def.active !== false,
        scope: def.user_id ? 'personal' : 'global',
        groups: [...(def.groups || [])],
        zones: [...(def.zones || [])],
        spaces,
      };
      this.editor.zoneValid = (def.zones || []).map(() => true);

      this.editor.active = true;
      this.loadReferenceData();
    },

    closeEditor() {
      if (this.editor.dirty) {
        this.discardConfirm.show = true;
        return;
      }
      this.editor.active = false;
    },

    discardChanges() {
      this.discardConfirm.show = false;
      this.editor.active = false;
    },

    async loadReferenceData() {
      try {
        const res = await fetch('/api/templates');
        if (res.ok) {
          const data = await res.json();
          this.editor.templates = data.templates || [];
          this.editor.form.spaces.forEach(s => {
            if (s.template_id && !s.template_name) {
              const tmpl = this.editor.templates.find(t => t.template_id === s.template_id);
              if (tmpl) s.template_name = tmpl.name;
            }
          });
        }
      } catch (e) { /* ignore */ }

      try {
        const res = await fetch('/api/scripts');
        if (res.ok) {
          const data = await res.json();
          this.editor.scripts = data.scripts || [];
          this.editor.form.spaces.forEach(s => {
            if (s.startup_script_id && !s.startup_script_name) {
              const script = this.editor.scripts.find(sc => sc.script_id === s.startup_script_id);
              if (script) s.startup_script_name = script.name;
            }
          });
        }
      } catch (e) { /* ignore */ }

      try {
        const res = await fetch('/api/groups');
        if (res.ok) {
          const data = await res.json();
          this.editor.groups = data.groups || [];
        }
      } catch (e) { /* ignore */ }
    },

    // ---- Component operations ----
    addComponent() {
      const idx = this.editor.form.spaces.length;
      const comp = blankComponent();
      comp.name = 'space-' + (idx + 1);
      this.editor.form.spaces.push(comp);
      this.markDirty();
      this.openComponentEditor(idx);
    },

    deleteComponent(ni) {
      const name = this.editor.form.spaces[ni].name;

      this.editor.form.spaces.forEach(s => {
        s.depends_on = (s.depends_on || []).filter(d => d !== name);
      });

      this.editor.form.spaces.forEach(s => {
        s.port_forwards = (s.port_forwards || []).filter(pf => pf.to_space !== name);
      });

      this.editor.form.spaces.splice(ni, 1);
      this.markDirty();
    },

    confirmDeleteComponent(ni) {
      this.deleteCompConfirm.index = ni;
      this.deleteCompConfirm.show = true;
    },

    executeDeleteComponent() {
      const ni = this.deleteCompConfirm.index;
      this.deleteCompConfirm.show = false;
      if (ni !== null && ni !== undefined) {
        this.deleteComponent(ni);
      }
    },

    // ---- Component editor modal ----
    async openComponentEditor(idx) {
      const space = this.editor.form.spaces[idx];
      if (!space) return;
      this.componentEditor.index = idx;

      let templateFields = [];
      // Fetch full template details to get custom_fields
      if (space.template_id) {
        try {
          const res = await fetch('/api/templates/' + space.template_id);
          if (res.ok) {
            const details = await res.json();
            templateFields = details.custom_fields || [];
          }
        } catch (e) { /* ignore */ }
      }

      // Build custom fields from template definitions, preserving existing values
      const existingFields = space.custom_fields || [];
      const custom_fields = templateFields.map(tf => {
        const existing = existingFields.find(ef => ef.name === tf.name);
        return {
          name: tf.name,
          description: tf.description || '',
          value: existing ? existing.value : '',
        };
      });

      // Build list of other space names for port forward target dropdowns
      const targetOptions = this.editor.form.spaces
        .filter((s, i) => i !== idx && s.name)
        .map(s => s.name);

      this.componentEditor.form = {
        name: space.name,
        template_id: space.template_id,
        template_name: space.template_name,
        description: space.description || '',
        shell: space.shell || '',
        startup_script_id: space.startup_script_id || '',
        startup_script_name: space.startup_script_name || '',
        depends_on: [...(space.depends_on || [])],
        custom_fields,
        port_forwards: (space.port_forwards || []).map(pf => ({ ...pf })),
        template_custom_fields: templateFields,
        _targetOptions: targetOptions,
      };
      this.componentEditor.show = true;
      this.$nextTick(() => { this.$dispatch('sync-comp-pickers'); });
    },

    saveComponentEditor() {
      const idx = this.componentEditor.index;
      if (idx === null || idx === undefined) return;
      const space = this.editor.form.spaces[idx];
      if (!space) return;

      const form = this.componentEditor.form;

      // Update name and clean up references if name changed
      const oldName = space.name;
      const newName = form.name;
      if (oldName !== newName && oldName) {
        this.editor.form.spaces.forEach(s => {
          s.depends_on = (s.depends_on || []).map(d => d === oldName ? newName : d);
          (s.port_forwards || []).forEach(pf => {
            if (pf.to_space === oldName) pf.to_space = newName;
          });
        });
      }

      space.name = form.name;
      space.template_id = form.template_id;
      space.template_name = form.template_name;
      space.description = form.description;
      space.shell = form.shell;
      space.startup_script_id = form.startup_script_id;
      space.startup_script_name = form.startup_script_name;
      space.depends_on = [...form.depends_on];
      space.custom_fields = form.custom_fields.filter(cf => cf.name);
      space.port_forwards = form.port_forwards.filter(pf => pf.to_space);

      if (document.activeElement) document.activeElement.blur();
      this.componentEditor.show = false;
      this.markDirty();
      this.autoValidate();
    },

    cancelComponentEditor() {
      if (document.activeElement) document.activeElement.blur();
      this.componentEditor.show = false;
    },

    toggleDependency(spaceName) {
      const deps = this.componentEditor.form.depends_on;
      const idx = deps.indexOf(spaceName);
      if (idx >= 0) {
        deps.splice(idx, 1);
      } else {
        deps.push(spaceName);
      }
      this.markDirty();
    },

    addPortForwardToEditor() {
      this.componentEditor.form.port_forwards.push({ to_space: '', local_port: 0, remote_port: 0 });
      this.markDirty();
    },

    removePortForwardFromEditor(pfIdx) {
      this.componentEditor.form.port_forwards.splice(pfIdx, 1);
      this.markDirty();
    },

    initPfRow(el, pf) {
      // Populate the target <select> options from data.
      // Runs once on init (before x-model sets the value), so the
      // correct option exists for Alpine to select.
      const select = el.querySelector('select');
      if (!select) return;
      const targets = this.componentEditor.form._targetOptions || [];
      for (const name of targets) {
        const opt = document.createElement('option');
        opt.value = name;
        opt.textContent = name;
        if (name === pf.to_space) opt.selected = true;
        select.appendChild(opt);
      }
    },

    async templateSelectedInEditor(template) {
      // Fetch full template details to get custom_fields
      try {
        const res = await fetch('/api/templates/' + template.template_id);
        if (res.ok) {
          const details = await res.json();
          const templateFields = details.custom_fields || [];
          this.componentEditor.form.custom_fields = templateFields.map(tf => ({
            name: tf.name,
            description: tf.description || '',
            value: '',
          }));
          this.componentEditor.form.template_custom_fields = templateFields;
        }
      } catch (e) {
        this.componentEditor.form.custom_fields = [];
        this.componentEditor.form.template_custom_fields = [];
      }
    },

    otherComponentsForEditor() {
      const idx = this.componentEditor.index;
      return this.editor.form.spaces.filter((s, i) => i !== idx && s.name);
    },

    // ---- Save flow ----
    buildRequestPayload() {
      return {
        name: this.editor.form.name,
        description: this.editor.form.description,
        active: this.editor.form.active,
        scope: this.editor.form.scope,
        groups: this.editor.form.groups,
        zones: this.editor.form.zones.filter(z => z.trim()),
        spaces: this.editor.form.spaces.map(s => ({
          name: s.name,
          template_id: s.template_id,
          description: s.description,
          shell: s.shell,
          startup_script_id: s.startup_script_id,
          depends_on: s.depends_on || [],
          custom_fields: (s.custom_fields || []).filter(cf => cf.name),
          port_forwards: (s.port_forwards || []).filter(pf => pf.to_space),
        })),
      };
    },

    async validateDefinition() {
      this.editor.validating = true;
      this.editor.errors = [];
      try {
        const payload = this.buildRequestPayload();
        const res = await fetch('/api/stack-definitions/validate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (res.ok) {
          const data = await res.json();
          this.editor.valid = data.valid;
          if (!data.valid) {
            this.editor.errors = data.errors || [];
          } else {
            this.$dispatch("show-alert", { msg: "Validation passed", type: "success" });
          }
        }
      } catch (e) { /* ignore */ }
      this.editor.validating = false;
    },

    async saveDefinition() {
      this.editor.saving = true;
      this.editor.errors = [];
      try {
        const payload = this.buildRequestPayload();

        // Validate before save
        const valRes = await fetch('/api/stack-definitions/validate', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload),
        });
        if (valRes.ok) {
          const valData = await valRes.json();
          if (!valData.valid) {
            this.editor.valid = false;
            this.editor.errors = valData.errors || [];
            this.editor.saving = false;
            return;
          }
        }

        let res;
        if (this.editor.defId) {
          res = await fetch(`/api/stack-definitions/${this.editor.defId}`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          });
        } else {
          res = await fetch('/api/stack-definitions', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
          });
        }

        if (res.status === 401) {
          window.location.href = "/logout";
          return;
        }

        if (res.ok || res.status === 201) {
          const data = await res.json().catch(() => ({}));
          if (data.stack_definition_id) {
            this.editor.defId = data.stack_definition_id;
          }
          this.editor.dirty = false;
          this.editor.valid = true;
          this.editor.active = false;
          this.$dispatch("show-alert", { msg: "Stack definition saved", type: "success" });
          this.getDefinitions();
        } else {
          const err = await res.json().catch(() => ({}));
          this.editor.errors = [{ field: '', message: err.error || 'Save failed' }];
        }
      } catch (e) {
        this.editor.errors = [{ field: '', message: 'Network error' }];
      }
      this.editor.saving = false;
    },

    _exportHelper(blob, filename) {
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = filename;
      a.click();
      URL.revokeObjectURL(url);
    },

    exportTOML() {
      const form = this.editor.form;
      const templates = this.editor.templates || [];
      const scripts = this.editor.scripts || [];
      const groups = this.editor.groups || [];

      let toml = '';
      toml += `name = ${tomlStr(form.name)}\n`;
      if (form.description) toml += `description = ${tomlStr(form.description)}\n`;
      toml += `scope = ${tomlStr(form.scope)}\n`;

      // Groups as names
      if (form.groups.length > 0) {
        const names = form.groups.map(gid => {
          const g = groups.find(gr => gr.group_id === gid);
          return g ? g.name : gid;
        });
        toml += `groups = [${names.map(tomlStr).join(', ')}]\n`;
      }

      // Zones
      if (form.zones.length > 0) {
        toml += `zones = [${form.zones.filter(z => z.trim()).map(tomlStr).join(', ')}]\n`;
      }

      // Spaces
      for (const space of form.spaces) {
        toml += `\n[[spaces]]\n`;
        toml += `name = ${tomlStr(space.name)}\n`;

        const tmpl = templates.find(t => t.template_id === space.template_id);
        if (tmpl) {
          toml += `template = ${tomlStr(tmpl.name)}\n`;
        }

        if (space.description) toml += `description = ${tomlStr(space.description)}\n`;
        if (space.shell) toml += `shell = ${tomlStr(space.shell)}\n`;

        if (space.startup_script_id) {
          const script = scripts.find(s => s.script_id === space.startup_script_id);
          if (script) {
            toml += `startup_script = ${tomlStr(script.name)}\n`;
          }
        }

        if (space.depends_on && space.depends_on.length > 0) {
          toml += `depends_on = [${space.depends_on.map(tomlStr).join(', ')}]\n`;
        }

        for (const cf of (space.custom_fields || []).filter(cf => cf.name)) {
          toml += `\n[[spaces.custom_fields]]\n`;
          toml += `name = ${tomlStr(cf.name)}\n`;
          toml += `value = ${tomlStr(cf.value || '')}\n`;
        }

        for (const pf of (space.port_forwards || []).filter(pf => pf.to_space)) {
          toml += `\n[[spaces.port_forwards]]\n`;
          toml += `to_space = ${tomlStr(pf.to_space)}\n`;
          toml += `local_port = ${pf.local_port}\n`;
          toml += `remote_port = ${pf.remote_port}\n`;
        }
      }

      this._exportHelper(new Blob([toml], { type: 'text/plain' }), (form.name || 'stack-definition') + '.toml');
    },

    exportJSON() {
      const payload = {
        name: this.editor.form.name,
        description: this.editor.form.description,
        scope: this.editor.form.scope,
        groups: this.editor.form.groups,
        zones: this.editor.form.zones.filter(z => z.trim()),
        spaces: this.editor.form.spaces.map(s => {
          const space = {
            name: s.name,
            template_id: s.template_id,
            description: s.description,
            shell: s.shell,
            startup_script_id: s.startup_script_id,
            depends_on: s.depends_on || [],
            custom_fields: (s.custom_fields || []).filter(cf => cf.name).map(cf => ({ name: cf.name, value: cf.value })),
            port_forwards: (s.port_forwards || []).filter(pf => pf.to_space),
          };
          return space;
        }),
      };
      const blob = new Blob([JSON.stringify(payload, null, 2)], { type: 'application/json' });
      this._exportHelper(blob, (this.editor.form.name || 'stack-definition') + '.json');
    },
  };
};

// ---- Inline autocomplete components for the builder ----

window.autocompleterTemplatePicker = function () {
  return {
    search: '',
    showList: false,
    selectedIndex: -1,
    _listboxId: Math.random().toString(36).slice(2),
    syncSearch() {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (comp && comp.componentEditor) {
        this.search = comp.componentEditor.form.template_name || '';
      }
    },
    get activeDescendant() {
      return this.selectedIndex >= 0 ? `${this._listboxId}-opt-${this.selectedIndex}` : '';
    },
    handleKeydown(e) {
      if (!this.showList) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        this.selectedIndex = this.selectedIndex < this.filteredTemplates.length - 1 ? this.selectedIndex + 1 : 0;
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        this.selectedIndex = this.selectedIndex > 0 ? this.selectedIndex - 1 : this.filteredTemplates.length - 1;
      } else if (e.key === 'Enter' && this.selectedIndex >= 0) {
        e.preventDefault();
        this.selectTemplate(this.filteredTemplates[this.selectedIndex]);
      } else if (e.key === 'Escape') {
        this.showList = false;
        this.selectedIndex = -1;
      }
    },
    handleFocus(e) {
      this.showList = true;
      this.selectedIndex = -1;
      if (e.target && this.search) e.target.select();
    },
    handleInput() {
      if (!this.search) {
        const root = this.$root.closest('[x-data]');
        const comp = Alpine.$data(root);
        if (comp && comp.componentEditor.index !== null) {
          comp.componentEditor.form.template_id = '';
          comp.componentEditor.form.template_name = '';
          comp.componentEditor.form.custom_fields = [];
          comp.componentEditor.form.template_custom_fields = [];
          comp.markDirty();
        }
      }
    },
    selectTemplate(t) {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (comp && comp.componentEditor.index !== null) {
        comp.componentEditor.form.template_id = t.template_id;
        comp.componentEditor.form.template_name = t.name;
        comp.templateSelectedInEditor(t);
        comp.markDirty();
      }
      this.search = t.name;
      this.showList = false;
      this.selectedIndex = -1;
    },
    get filteredTemplates() {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (!comp) return [];
      const term = this.search.toLowerCase();
      return (comp.editor.templates || []).filter(t =>
        t.name.toLowerCase().includes(term)
      );
    },
  };
};

window.autocompleterScriptPicker = function () {
  return {
    search: '',
    showList: false,
    selectedIndex: -1,
    _listboxId: Math.random().toString(36).slice(2),
    syncSearch() {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (comp && comp.componentEditor) {
        this.search = comp.componentEditor.form.startup_script_name || '';
      }
    },
    get activeDescendant() {
      return this.selectedIndex >= 0 ? `${this._listboxId}-opt-${this.selectedIndex}` : '';
    },
    handleKeydown(e) {
      if (!this.showList) return;
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        this.selectedIndex = this.selectedIndex < this.filteredScripts.length - 1 ? this.selectedIndex + 1 : 0;
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        this.selectedIndex = this.selectedIndex > 0 ? this.selectedIndex - 1 : this.filteredScripts.length - 1;
      } else if (e.key === 'Enter' && this.selectedIndex >= 0) {
        e.preventDefault();
        this.selectScript(this.filteredScripts[this.selectedIndex]);
      } else if (e.key === 'Escape') {
        this.showList = false;
        this.selectedIndex = -1;
      }
    },
    handleFocus(e) {
      this.showList = true;
      this.selectedIndex = -1;
      if (e.target && this.search) e.target.select();
    },
    handleInput() {
      if (!this.search) {
        const root = this.$root.closest('[x-data]');
        const comp = Alpine.$data(root);
        if (comp && comp.componentEditor.index !== null) {
          comp.componentEditor.form.startup_script_id = '';
          comp.componentEditor.form.startup_script_name = '';
          comp.markDirty();
        }
      }
    },
    selectScript(s) {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (comp && comp.componentEditor.index !== null) {
        comp.componentEditor.form.startup_script_id = s.script_id;
        comp.componentEditor.form.startup_script_name = s.name;
        comp.markDirty();
      }
      this.search = s.name;
      this.showList = false;
      this.selectedIndex = -1;
    },
    get filteredScripts() {
      const root = this.$root.closest('[x-data]');
      const comp = Alpine.$data(root);
      if (!comp) return [];
      const term = this.search.toLowerCase();
      return (comp.editor.scripts || []).filter(s =>
        s.name.toLowerCase().includes(term)
      );
    },
  };
};
