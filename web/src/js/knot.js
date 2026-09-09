import '../less/knot.css';

import Alpine from 'alpinejs';
import persist from '@alpinejs/persist';
import AlpineFloatingUI from "@awcodes/alpine-floating-ui";
import collapse from '@alpinejs/collapse';
import focus from '@alpinejs/focus';

import {} from './timezones.js';
import {} from './components/autocompleter.js';

import md5 from 'crypto-js/md5';

// SSE client for real-time updates
import './sse.js';

import './pages/initialUserForm.js';
import './pages/loginUserForm.js';
import './pages/userGroupForm.js';
import './pages/createTokenForm.js';
import './pages/apiTokensComponent.js';
import './pages/groupListComponent.js';
import './pages/rolesListComponent.js';
import './pages/userRolesForm.js';
import './pages/pluginsListComponent.js';
import './plugins/pluginPage.js';
import './pages/sessionsListComponent.js';
import './pages/templateListComponent.js';
import './pages/templateForm.js';
import './pages/userListComponent.js';
import './pages/userForm.js';
import './pages/templateVarListComponent.js';
import './pages/variableForm.js';
import './pages/volumeListComponent.js';
import './pages/volumeForm.js';
import './pages/spaceForm.js';
import './pages/spacesListComponent.js';
import './pages/spaceUsageComponent.js';
import './pages/usageComponent.js';
import './pages/tunnelsListComponent.js';
import './pages/auditLogComponent.js';
import './pages/clusterInfoComponent.js';
import './pages/scriptListComponent.js';
import './pages/scriptForm.js';
import './pages/aceEditorCompleter.js'; // sets window.AceEditorCompleter
import { scriptLibraries } from './pages/scriptCompletions.js';
import './pages/eventSinkListComponent.js';
import './pages/eventSinkForm.js';
import './pages/skillListComponent.js';
import './pages/skillForm.js';
import './pages/commandListComponent.js';
import './pages/commandForm.js';
import './pages/mcpServerListComponent.js';
import './pages/mcpServerForm.js';
import './pages/stackDefinitionBuilder.js';
import './pages/stackListComponent.js';

import './terminal.js';
import './movable-modal.js';
import './nav-starred.js';
import './search-palette.js';
import './form-dirty-guard.js';

window.Alpine = Alpine;
// Expose the bundled chart.js for plugin pages (plugins are trusted; they
// render inline with knot's own libraries and versions).
import Chart from 'chart.js/auto';
window.Chart = Chart;

// Switch the session to another user the current one may become (the
// profile menu's Switch User list) or back to the session's origin, then
// reload into it. The server answers with a bare status — it never
// redirects, so fetch cannot render a throwaway page — and refuses
// anything else, so a failed or forged call just lands home.
window.knotSwitchUser = function knotSwitchUser(userId) {
  fetch('/switch-user/' + encodeURIComponent(userId), { method: 'POST' })
    .then(() => { window.location.href = '/'; })
    .catch(() => { window.location.href = '/'; });
};

// Autocompleter for template custom fields bound to a plugin field
// handler: suggestions come from /api/plugins/field-handlers/<id>, the
// value stays free-typed so pick-or-create works.
// Ace-backed editor for textarea custom fields: seeded from the field's
// stored string, every change writes straight back to formData.
window.customFieldEditor = function customFieldEditor(index, language) {
  return {
    editor: null,
    formDataScope() {
      let scope = this.$root.parentElement;
      while (scope && !(scope._x_dataStack && scope._x_dataStack.some((d) => d.formData))) scope = scope.parentElement;
      return scope ? scope._x_dataStack.find((d) => d.formData) : null;
    },
    init() {
      this.$nextTick(() => {
        const data = this.formDataScope();
        let dark = JSON.parse(localStorage.getItem('_x_darkMode'));
        if (dark == null) dark = true;
        this.$root.id = `cf-editor-${index}`;
        this.editor = ace.edit(this.$root);
        this.editor.session.setValue((data && data.formData.custom_fields[index] && data.formData.custom_fields[index].value) || '');
        this.editor.session.on('change', () => {
          const d = this.formDataScope();
          if (d) {
            try { d.formData.custom_fields[index].value = this.editor.getValue(); } catch (e) { /* index race */ }
          }
        });
        this.editor.setTheme(dark ? 'ace/theme/github_dark' : 'ace/theme/github');
        // Language drives the ace mode; scriptling also gets the same
        // knot-aware completions the script editors use.
        const modes = { yaml: 'ace/mode/yaml', toml: 'ace/mode/toml', json: 'ace/mode/json', markdown: 'ace/mode/markdown', shell: 'ace/mode/sh', scriptling: 'ace/mode/python' };
        const mode = modes[language];
        if (mode) this.editor.session.setMode(mode);
        if (language === 'scriptling' && window.AceEditorCompleter) {
          window.AceEditorCompleter.setup(this.editor, scriptLibraries, { debug: false });
        }
        const scriptling = language === 'scriptling';
        this.editor.setOptions({
          printMargin: false,
          newLineMode: 'unix',
          tabSize: 2,
          wrap: scriptling ? false : true,
          enableBasicAutocompletion: scriptling,
          enableLiveAutocompletion: scriptling,
        });
      });
      window.addEventListener('theme-change', (e) => {
        if (this.editor) this.editor.setTheme(e.detail.dark_theme ? 'ace/theme/github_dark' : 'ace/theme/github');
      });
    },
  };
};

// Dropdown twin of the autocompleter for select custom fields: the same
// plugin field handler serves the options (key/text pairs or plain
// strings); the select stores the key and shows the text. Loaded once when
// the field renders.
window.fieldSelect = function fieldSelect(handlerId, staticOptions) {
  return {
    options: Array.isArray(staticOptions) && staticOptions.length ? staticOptions.map(String) : [],
    loaded: false,
    async init() {
      if (this.loaded || this.options.length) return;
      try {
        const response = await fetch(`/api/plugins/field-handlers/${encodeURIComponent(handlerId)}`, { cache: 'no-store' });
        if (response.ok) {
          const data = await response.json();
          const options = Array.isArray(data) ? data : data.options;
          this.options = options || [];
        }
      } catch (e) { /* keep empty */ }
      this.loaded = true;
    },
    optKey(option) {
      return typeof option === 'object' && option !== null ? (option.key ?? option.text ?? '') : String(option);
    },
    optText(option) {
      return typeof option === 'object' && option !== null ? (option.text || option.key) : String(option);
    },
  };
};

window.fieldAutocompleter = function fieldAutocompleter(handlerId, staticOptions) {
  return {
    search: '',
    options: Array.isArray(staticOptions) && staticOptions.length ? staticOptions.map(String) : [],
    loaded: false,
    showList: false,
    selectedIndex: -1,
    dropdownStyle: '',
    dropdownVisible: false,
    scrollListener: null,
    positionTimeout: null,
    optKey(o) { return typeof o === 'object' && o !== null ? o.key : o; },
    optText(o) { return typeof o === 'object' && o !== null ? (o.text || o.key) : o; },
    get filteredOptions() {
      const q = (this.search || '').toLowerCase();
      if (!q) return this.options;
      return this.options.filter((o) => this.optText(o).toLowerCase().includes(q) || this.optKey(o).toLowerCase().includes(q));
    },
    // The same fixed-position dropdown the icon search uses: the list
    // overlays the form (footer included) instead of being clipped by the
    // modal's scroll body, flips above when the viewport is tight, and
    // follows the input through scrolls.
    positionDropdown() {
      const input = (this.$refs && this.$refs.cfInput) || (this.$root && this.$root.querySelector('input'));
      const dropdown = (this.$refs && this.$refs.dropdown) || (this.$root && (this.$root.querySelector('[x-ref="dropdown"]') || this.$root.querySelector('.fixed')));
      if (!input || !dropdown) return;
      const rect = input.getBoundingClientRect();
      const viewportHeight = window.innerHeight;
      const dropdownHeight = 160;
      const gap = 4;
      const spaceBelow = viewportHeight - rect.bottom;
      const spaceAbove = rect.top;
      if (spaceBelow >= dropdownHeight || spaceBelow >= spaceAbove) {
        this.dropdownStyle = `top: ${rect.bottom + gap}px; left: ${rect.left}px; width: ${rect.width}px;`;
      } else {
        this.dropdownStyle = `bottom: ${viewportHeight - rect.top + gap}px; left: ${rect.left}px; width: ${rect.width}px;`;
      }
      this.dropdownVisible = true;
    },

    schedulePositioning() {
      this.positionDropdown();
      requestAnimationFrame(() => this.positionDropdown());
      if (this.positionTimeout) clearTimeout(this.positionTimeout);
      this.positionTimeout = setTimeout(() => this.positionDropdown(), 220);
    },

    formDataCtx() {
      const idx = this.$root.getAttribute('data-cf-index');
      if (idx === null) return null;
      let scope = this.$root.parentElement;
      while (scope && !(scope._x_dataStack && scope._x_dataStack.some((d) => d.formData))) scope = scope.parentElement;
      const data = scope ? scope._x_dataStack.find((d) => d.formData) : null;
      return data ? { data, index: Number(idx) } : null;
    },

    init() {
      // Load eagerly: a stored key must resolve to its display text on
      // form load, before the user ever focuses the field.
      this.loadOptions();
    },

    async handleFocus() {
      this.showList = true;
      this.selectedIndex = -1;
      this.schedulePositioning();
      if (!this.scrollListener) {
        this.scrollListener = () => { if (this.showList) this.positionDropdown(); };
        document.addEventListener('scroll', this.scrollListener, true);
      }
      if (!this.loaded) this.loadOptions();
    },

    async loadOptions() {
      if (this.loaded || this.options.length) {
        // Manual (static) options need no fetch — but the stored-key
        // display resolution below still applies to them.
        if (!this.loaded) {
          this.loaded = true;
          this.resolveStoredDisplay();
        }
        return;
      }
      try {
        const response = await fetch(`/api/plugins/field-handlers/${encodeURIComponent(handlerId)}`, { cache: 'no-store' });
        if (response.ok) {
          const data = await response.json();
          const options = Array.isArray(data) ? data : data.options;
          this.options = options || [];
        }
      } catch (e) { /* keep empty */ }
      this.loaded = true;
      this.resolveStoredDisplay();
    },
    // The field displays TEXT; the store holds the KEY. If the stored
    // value is a key we now know the text for, resolve the display.
    resolveStoredDisplay() {
      const ctx = this.formDataCtx();
      if (ctx) {
        const stored = ctx.data.formData.custom_fields[ctx.index].value;
        const match = this.options.find((o) => this.optKey(o) === stored);
        if (match && this.search === stored) this.search = this.optText(match);
      }
    },
    handleKeydown(event) {
      if (!this.showList) return;
      if (event.key === 'ArrowDown' || event.key === 'ArrowUp') this.schedulePositioning();
      const filtered = this.filteredOptions;
      if (event.key === 'ArrowDown') {
        event.preventDefault();
        this.selectedIndex = this.selectedIndex < filtered.length - 1 ? this.selectedIndex + 1 : 0;
      } else if (event.key === 'ArrowUp') {
        event.preventDefault();
        this.selectedIndex = this.selectedIndex > 0 ? this.selectedIndex - 1 : filtered.length - 1;
      } else if (event.key === 'Enter' && this.selectedIndex >= 0 && filtered[this.selectedIndex] !== undefined) {
        event.preventDefault();
        this.selectOption(filtered[this.selectedIndex]);
      } else if (event.key === 'Escape') {
        this.showList = false;
      }
    },
    selectOption(option) {
      this.search = this.optText(option);
      this.showList = false;
      this.selectedIndex = -1;
      // Store the KEY directly on the form's data - no input events, so
      // nothing can race the display back to the key.
      const ctx = this.formDataCtx();
      if (ctx) {
        try { ctx.data.formData.custom_fields[ctx.index].value = this.optKey(option); } catch (e) { /* index race */ }
      }
    },
  };
};
Alpine.plugin(persist);
Alpine.plugin(AlpineFloatingUI);
Alpine.plugin(focus);
Alpine.plugin(collapse);
Alpine.start();

window.MD5 = function(str) {
  return md5(str).toString();
}
