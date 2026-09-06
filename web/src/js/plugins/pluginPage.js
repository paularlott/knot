// Plugin page runtime: the handler returns a LAYOUT - rows of columns,
// each column declaring its type, data handler and refresh. This component
// renders the shell (row grid, column cards, loaders), fetches each
// column's data from the same URL (?_col=<id>), and renders it with the
// block renderers from pluginBlocks.js. Form columns POST to their own
// _col endpoint; the response envelope (status/message/field_errors/
// refresh) drives notifications and column refreshes. All wiring is
// delegated from the page root so patched columns never re-bind.

import { renderBlock, initPluginChart } from './pluginBlocks.js';

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

const SPAN = { 1: 'col-span-1', 2: 'col-span-2', 3: 'col-span-3', 4: 'col-span-4' };

window.pluginPage = function pluginPage(url) {
  return {
    url,
    params: '',
    timers: {},
    _ac: new WeakMap(),

    init() {
      const embedded = document.getElementById('plugin-document');
      const region = this.region();
      if (!embedded || !region) return;
      this.params = new URLSearchParams(window.location.search).toString();
      let doc = null;
      try { doc = JSON.parse(embedded.textContent); } catch (e) { /* bad doc */ }
      if (doc) this.render(doc);
      this.bindRoot(region);
    },

    region() {
      return this.$root.querySelector('[data-plugin-region]');
    },

    // ---- layout ------------------------------------------------------

    render(doc) {
      const region = this.region();
      if (!region) return;
      region.innerHTML = '';
      this.clearTimers();
      if (doc.error) {
        region.appendChild(el('div', 'rounded-lg border border-red-200 dark:border-red-900 p-4 text-sm text-red-700 dark:text-red-300', doc.error));
        return;
      }
      (doc.rows || []).forEach((row) => region.appendChild(this.renderRow(row)));
    },

    renderRow(row) {
      const card = row.style === 'card';
      const columnsHost = card
        ? el('div', 'bg-white border border-gray-200 rounded-lg shadow-xs dark:border-gray-700 dark:bg-gray-800 p-4 grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-4')
        : el('div', 'grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 xl:gap-4');
      const host = el('div', 'space-y-3');
      if (row.title) host.appendChild(el('h2', 'text-lg font-semibold text-gray-900 dark:text-white break-words', row.title));
      host.appendChild(columnsHost);
      (row.columns || []).forEach((column) => columnsHost.appendChild(this.renderColumn(column, card)));
      return host;
    },

    renderColumn(column, plain) {
      // Card rows tint their columns (the inner-card treatment); default
      // rows card each column in white.
      const wrap = el('div', `${SPAN[column.width] || SPAN[4]} min-w-0 flex flex-col space-y-3${plain ? ' bg-gray-50 dark:bg-gray-900/40 rounded-lg p-4' : ' bg-white border border-gray-200 rounded-lg shadow-xs dark:border-gray-700 dark:bg-gray-800 p-4'}`);
      wrap.dataset.colId = column.id;
      wrap.dataset.colHandler = column.handler || '';
      if (column.title) wrap.appendChild(el('h3', 'text-sm font-semibold text-gray-900 dark:text-white break-words', column.title));
      const body = el('div', 'space-y-3 flex-1 flex flex-col');
      body.dataset.colBody = '1';
      wrap.appendChild(body);
      this.showLoader(body);
      if (column.handler) this.fetchColumn(column, body);
      else this.renderColumnData(column, {}, body);
      if (column.refresh) {
        this.timers[column.id] = setInterval(() => {
          if (!this.userReading()) this.fetchColumn(column, body, true);
        }, column.refresh * 1000);
      }
      return wrap;
    },

    showLoader(body) {
      body.innerHTML = '';
      const holder = el('div', 'flex items-center p-2 text-base w-full text-gray-900 dark:text-white');
      holder.setAttribute('role', 'status');
      const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
      svg.setAttribute('class', 'w-6 h-6 mr-2 animate-spin');
      svg.setAttribute('fill', 'none');
      svg.setAttribute('viewBox', '0 0 24 24');
      svg.setAttribute('stroke-width', '1.5');
      svg.setAttribute('stroke', 'currentColor');
      svg.innerHTML = '<path stroke-linecap="round" stroke-linejoin="round" d="M16.023 9.348h4.992v-.001M2.985 19.644v-4.992m0 0h4.992m-4.993 0 3.181 3.183a8.25 8.25 0 0 0 13.803-3.7M4.031 9.865a8.25 8.25 0 0 1 13.803-3.7l3.181 3.182m0-4.991v4.99"/>';
      holder.appendChild(svg);
      holder.appendChild(el('span', '', 'Loading …'));
      body.appendChild(holder);
    },

    // knot's auth middleware answers 503 on transient store hiccups
    // specifically so clients retry instead of failing; honour that.
    async fetchWithRetry(url, options, attempts = 3) {
      let delay = 300;
      for (let i = 0; ; i += 1) {
        const response = await fetch(url, options);
        if (response.status !== 503 || i >= attempts - 1) return response;
        await new Promise((r) => setTimeout(r, delay));
        delay *= 3;
      }
    },

    async fetchColumn(column, body, isRefresh) {
      body.closest('[data-col-id]')?.setAttribute('aria-busy', 'true');
      try {
        const qs = this.params ? `${this.params}&_json=1&_col=${encodeURIComponent(column.handler)}` : `_json=1&_col=${encodeURIComponent(column.handler)}`;
        const response = await this.fetchWithRetry(`${this.url}?${qs}`, { headers: { 'Content-Type': 'application/json' } });
        if (response.status !== 200) throw new Error(`HTTP ${response.status}`);
        const text = await response.text();
        let data;
        try {
          data = JSON.parse(text);
        } catch (e) {
          // HTML means a redirect (auth expired) or a stale server: never
          // silent - surface it in the panel and the console.
          // eslint-disable-next-line no-console
          console.error(`column ${column.id}: non-JSON response`, text.slice(0, 120));
          throw new Error('non-JSON response (session or server?)');
        }
        this.renderColumnData(column, data, body, isRefresh);
      } catch (e) {
        body.innerHTML = '';
        body.appendChild(el('div', 'rounded-lg border border-red-200 dark:border-red-900 p-3 text-sm text-red-700 dark:text-red-300', 'This panel failed to load.'));
      } finally {
        body.closest('[data-col-id]')?.setAttribute('aria-busy', 'false');
      }
    },

    // Map a column payload to the block renderer for its type.
    renderColumnData(column, data, body, isRefresh) {
      // Capture any live chart BEFORE clearing the body: the canvas is
      // destroyed by the clear, and with it the in-place update path.
      const liveCanvas = body.querySelector('canvas');
      const liveChart = liveCanvas && window.Chart ? window.Chart.getChart(liveCanvas) : null;
      body.innerHTML = '';
      if (data && data.error && !data.columns) {
        body.appendChild(el('div', 'rounded-lg border border-red-200 dark:border-red-900 p-3 text-sm text-red-700 dark:text-red-300', `This panel failed to load: ${data.error}`));
        return;
      }
      try {
        if (column.type === 'stat') {
          body.appendChild(renderBlock({ type: 'stat', ...(data || {}) }));
        } else if (column.type === 'chart') {
          // Refreshes update the existing chart in place: replacing the
          // canvas would restart the chart (and its animations) every tick.
          const chart = isRefresh ? liveChart : null;
          if (chart) {
            // Unchanged data must not move: only changed values animate.
            const sameLabels = JSON.stringify(chart.data.labels) === JSON.stringify(data.labels || []);
            const sameData = sameLabels && (data.datasets || []).every((ds, i) => {
              const current = chart.data.datasets[i];
              return current && JSON.stringify(current.data) === JSON.stringify(ds.data);
            }) && chart.data.datasets.length === (data.datasets || []).length;
            if (sameData) {
              body.appendChild(liveCanvas);
              return;
            }
            chart.data.labels = data.labels || [];
            chart.data.datasets = (data.datasets || []).map((ds, i) => ({
              ...chart.data.datasets[i],
              label: ds.name,
              data: ds.data,
              borderColor: ds.color || chart.data.datasets[i].borderColor,
              backgroundColor: ds.color || chart.data.datasets[i].backgroundColor,
            }));
            while (chart.data.datasets.length > (data.datasets || []).length) chart.data.datasets.pop();
            body.appendChild(liveCanvas);
            chart.update('none');
            return;
          }
          const canvas = el('canvas');
          body.appendChild(canvas);
          initPluginChart(canvas, data);
        } else if (column.type === 'table') {
          body.appendChild(this.renderTable(column, data));
        } else if (column.type === 'form') {
          this.renderForm(column, data, body);
        } else if (column.type === 'html' || column.type === 'markdown') {
          body.appendChild(renderBlock({ type: column.type === 'markdown' ? 'markdown' : 'html', html: data.html || String(data) }));
        } else if (column.type === 'bar' && Array.isArray(data)) {
          data.forEach((bar) => body.appendChild(renderBlock({ type: 'bar', ...bar })));
        } else {
          body.appendChild(renderBlock({ type: column.type, ...(data || {}) }));
        }
      } catch (e) {
        body.appendChild(el('div', 'rounded-lg border border-red-200 dark:border-red-900 p-3 text-sm text-red-700 dark:text-red-300', `Panel render failed: ${e.message}`));
      }
    },

    renderTable(column, data) {
      const tableBlock = renderBlock({ type: 'table', columns: data.columns, rows: data.rows });
      if (column.actions && column.actions.length && data.rows) {
        const head = tableBlock.querySelector('[data-head]');
        if (head) {
          const th = el('th', 'px-4 py-3 text-xs font-medium tracking-wider text-left uppercase');
          th.scope = 'col';
          th.textContent = '';
          head.appendChild(th);
        }
        const bodyEl = tableBlock.querySelector('[data-body]');
        [...bodyEl.rows].forEach((tr, i) => {
          const td = tr.insertCell(-1);
          td.className = 'px-4 py-3 whitespace-nowrap';
          const key = data.rows[i] && (data.rows[i].id || data.rows[i].name || '');
          column.actions.forEach((action) => {
            const btn = el('button', action.style === 'danger'
              ? 'inline-flex items-center rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800/45 text-red-700 dark:text-red-400 px-2.5 py-1.5 text-xs font-medium transition-colors hover:bg-slate-100 dark:hover:bg-slate-700/70 mr-1'
              : 'inline-flex items-center rounded-lg border border-slate-300 dark:border-slate-700 bg-white dark:bg-slate-800/45 text-blue-700 dark:text-blue-400 px-2.5 py-1.5 text-xs font-medium transition-colors hover:bg-slate-100 dark:hover:bg-slate-700/70 mr-1', action.label);
            btn.type = 'button';
            btn.dataset.action = action.action || '';
            btn.dataset.key = String(key);
            btn.dataset.confirm = action.confirm || '';
            td.appendChild(btn);
          });
        });
      }
      return tableBlock;
    },

    renderForm(column, data, body) {
      const form = renderBlock({
        type: 'form',
        fields: data.fields,
        submit: data.submit,
        cancel: data.cancel,
      }, {
        // dynamic_options: the column's own handler is the suggestion
        // source - the client asks it with _data=<field name> and it
        // returns {"options": [...]} (key/text pairs or plain strings).
        onDynamicOptions: (input, field) => this.fetchDynamicOptions(column, input),
      });
      form.dataset.colId = column.id;
      form.dataset.colHandler = column.handler || '';
      body.appendChild(form);
    },

    async fetchDynamicOptions(column, input) {
      try {
        const dataParam = `&_data=${encodeURIComponent(input.name)}`;
        const qs = `_json=1&_col=${encodeURIComponent(column.handler)}${dataParam}`;
        const data = await this.fetchWithRetry(`${this.url}?${qs}`, { headers: { 'Content-Type': 'application/json' } }).then((r) => r.json());
        let options = Array.isArray(data) ? data : data && data.options;
        if (!Array.isArray(options)) options = [];
        input.dataset.pbOptions = JSON.stringify(options.map((o) => (typeof o === 'object' && o !== null ? { key: o.key, text: o.text || o.key } : { key: String(o), text: String(o) })));
      } catch (e) { /* keep whatever fixed options the field declared */
      }
    },

    // ---- forms: POST to the column endpoint, handle the envelope ------

    async submitForm(form) {
      const columnId = form.dataset.colHandler || form.closest('[data-col-handler]')?.dataset.colHandler;
      const body = new URLSearchParams(new FormData(form)).toString();
      const wrap = form.closest('[data-col-id]') || form.parentElement;
      wrap?.setAttribute('aria-busy', 'true');
      try {
        const response = await this.fetchWithRetry(`${this.url}?_json=1&_col=${encodeURIComponent(columnId)}`, {
          method: 'POST',
          headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
          body,
        });
        const result = await response.json();
        if (result && result.rows !== undefined) {
          // Full layout returned: re-render the page.
          this.render(result);
          return;
        }
        this.handleEnvelope(form, result);
      } catch (e) {
        this.toast('The action failed to submit.', 'error');
      } finally {
        wrap?.setAttribute('aria-busy', 'false');
      }
    },

    handleEnvelope(form, envelope) {
      if (!envelope || envelope.status === undefined) return;
      if (envelope.status === 'ok') {
        this.toast(envelope.message || 'Done.', 'ok');
        // Filter forms: the submitted fields become the page's params so
        // refreshed columns fetch with the new filters.
        if (form) {
          try { this.params = new URLSearchParams(new FormData(form)).toString(); } catch (e) { /* form detached */ }
        }
        // Refresh the columns the handler names (or all).
        if (envelope.refresh) {
          const targets = envelope.refresh === true ? null : envelope.refresh;
          this.region().querySelectorAll('[data-col-id]').forEach((node) => {
            if (!targets || targets.includes(node.dataset.colId)) {
              const body = node.querySelector('[data-col-body]');
              const column = this.columnDef(node.dataset.colId);
              if (column && column.handler) { this.showLoader(body); this.fetchColumn(column, body, true); }
            }
          });
        }
        return;
      }
      // error: message + per-field errors mapped back onto the form
      if (envelope.message) this.toast(envelope.message, 'error');
      if (form && envelope.field_errors) {
        Object.entries(envelope.field_errors).forEach(([name, message]) => {
          const input = form.querySelector(`[name="${CSS.escape(name)}"]`);
          if (input) {
            input.classList.add('form-field-error');
            const err = el('div', 'error-message', message);
            input.insertAdjacentElement('afterend', err);
          }
        });
      }
    },

    toast(message, kind) {
      let host = document.querySelector('[data-plugin-status]');
      if (!host) return window.alert ? console.log(`[plugin] ${kind}: ${message}`) : null;
      host.textContent = message;
      host.dataset.kind = kind;
      clearTimeout(this._toastTimer);
      this._toastTimer = setTimeout(() => { host.textContent = ''; }, 4000);
    },

    columnDef(id) {
      const embedded = document.getElementById('plugin-document');
      try {
        const doc = JSON.parse(embedded.textContent);
        for (const row of doc.rows || []) {
          for (const column of row.columns || []) {
            if (column.id === id) return column;
          }
        }
      } catch (e) { /* stale */ }
      return null;
    },

    userReading() {
      const region = this.region();
      return region ? (region.contains(document.activeElement) && document.activeElement !== document.body) || region.matches(':hover') : false;
    },

    clearTimers() {
      Object.values(this.timers).forEach(clearInterval);
      this.timers = {};
    },

    // ---- delegated wiring ---------------------------------------------

    acState(input) {
      if (!this._ac.has(input)) this._ac.set(input, { selected: -1, open: false, hideTimer: null });
      return this._ac.get(input);
    },

    // ---- autocompleter (delegated; fixed-position dropdown) -----------

    acOptions(input) {
      try { return JSON.parse(input.dataset.pbOptions || '[]'); } catch (e) { return []; }
    },

    acText(option) { return typeof option === 'object' && option !== null ? (option.text || option.key) : option; },

    acFiltered(input) {
      const q = (input.value || '').toLowerCase();
      const options = this.acOptions(input);
      if (!q) return options;
      return options.filter((o) => this.acText(o).toLowerCase().includes(q));
    },

    acList(input) {
      return input.closest('[data-ac]').querySelector('[data-ac-list]');
    },

    acPosition(input, list) {
      const rect = input.getBoundingClientRect();
      list.style.position = 'fixed';
      list.style.left = `${Math.round(rect.left)}px`;
      list.style.width = `${Math.round(rect.width)}px`;
      list.style.top = '';
      const below = window.innerHeight - rect.bottom;
      if (below < list.offsetHeight + 8 && rect.top > list.offsetHeight + 8) {
        list.style.top = `${Math.round(rect.top - list.offsetHeight - 6)}px`;
      } else {
        list.style.top = `${Math.round(rect.bottom + 6)}px`;
      }
    },

    acRender(input) {
      const list = this.acList(input);
      const ul = list.querySelector('ul');
      const filtered = this.acFiltered(input);
      const state = this.acState(input);
      ul.innerHTML = '';
      filtered.forEach((option, index) => {
        const li = el('li', 'my-1 cursor-pointer nav-item text-sm' + (index === state.selected ? ' bg-blue-100 dark:bg-blue-900' : ''), this.acText(option));
        li.setAttribute('role', 'option');
        li.dataset.acOption = '1';
        li.id = `${input.id || 'ac'}-opt-${index}`;
        if (index === state.selected) {
          li.setAttribute('aria-selected', 'true');
          input.setAttribute('aria-activedescendant', li.id);
        } else {
          li.setAttribute('aria-selected', 'false');
        }
        ul.appendChild(li);
      });
      input.setAttribute('aria-expanded', filtered.length ? 'true' : 'false');
    },

    acShow(input) {
      this.acRender(input);
      const list = this.acList(input);
      list.style.display = 'block';
      this.acPosition(input, list);
      requestAnimationFrame(() => this.acPosition(input, this.acList(input)));
      this.acState(input).open = true;
    },

    acHide(input) {
      const list = this.acList(input);
      if (list) list.style.display = 'none';
      input.setAttribute('aria-expanded', 'false');
      this.acState(input).open = false;
    },

    acSelect(input, option) {
      input.value = this.acText(option);
      this.acHide(input);
    },

    bindRoot(region) {
      region.addEventListener('submit', (event) => {
        const form = event.target.closest('form[data-plugin-form]');
        if (!form) return;
        event.preventDefault();
        this.submitForm(form);
      });
      region.addEventListener('click', (event) => {
        const btn = event.target.closest('button[data-action]');
        if (!btn) return;
        const run = () => {
          const columnId = btn.closest('[data-col-handler]')?.dataset.colHandler || '';
          const body = new URLSearchParams({ action: btn.dataset.action, key: btn.dataset.key }).toString();
          btn.disabled = true;
          this.fetchWithRetry(`${this.url}?_json=1&_col=${encodeURIComponent(columnId)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body,
          }).then((r) => r.json()).then((result) => {
            btn.disabled = false;
            this.handleEnvelope(null, result);
          }).catch(() => { btn.disabled = false; this.toast('The action failed to submit.', 'error'); });
        };
        if (btn.dataset.confirm) {
          // Two-step inline confirm: click arms, click again runs.
          if (btn.dataset.armed !== '1') {
            btn.dataset.armed = '1';
            btn.dataset.label = btn.textContent;
            btn.textContent = 'Confirm?';
            setTimeout(() => {
              if (btn.dataset.armed === '1') {
                btn.dataset.armed = '';
                btn.textContent = btn.dataset.label || btn.textContent;
              }
            }, 3000);
            return;
          }
          btn.dataset.armed = '';
        }
        run();
      });
      region.addEventListener('input', (event) => {
        const input = event.target;
        if (!input.matches || !input.matches('[data-ac-input]')) return;
        this.acState(input).selected = -1;
        // Typing always (re)opens the list: the native search clear also
        // fires input without focus, and the list must recover from that.
        this.acShow(input);
      });
      region.addEventListener('focusin', (event) => {
        if (event.target.matches && event.target.matches('[data-ac-input]')) this.acShow(event.target);
      });
      region.addEventListener('focusout', (event) => {
        if (!event.target.matches || !event.target.matches('[data-ac-input]')) return;
        const input = event.target;
        clearTimeout(this.acState(input).hideTimer);
        this.acState(input).hideTimer = setTimeout(() => this.acHide(input), 150);
      });
      region.addEventListener('keydown', (event) => {
        if (!event.target.matches || !event.target.matches('[data-ac-input]')) return;
        const input = event.target;
        const state = this.acState(input);
        const filtered = this.acFiltered(input);
        if (event.key === 'ArrowDown') {
          event.preventDefault();
          state.selected = state.selected < filtered.length - 1 ? state.selected + 1 : 0;
          this.acShow(input);
        } else if (event.key === 'ArrowUp') {
          event.preventDefault();
          state.selected = state.selected > 0 ? state.selected - 1 : filtered.length - 1;
          this.acShow(input);
        } else if (event.key === 'Enter' && state.selected >= 0 && filtered[state.selected] !== undefined) {
          event.preventDefault();
          this.acSelect(input, filtered[state.selected]);
        } else if (event.key === 'Escape') {
          event.stopPropagation();
          this.acHide(input);
        }
      });
      region.addEventListener('mousedown', (event) => {
        const option = event.target.closest('[data-ac-option]');
        if (!option) return;
        event.preventDefault();
        const input = option.closest('[data-ac]').querySelector('[data-ac-input]');
        const match = this.acFiltered(input).find((o) => this.acText(o) === option.textContent);
        this.acSelect(input, match !== undefined ? match : option.textContent);
        input.focus({ preventScroll: true });
      });
      region.addEventListener('scroll', () => {
        region.querySelectorAll('[data-ac-input]').forEach((input) => {
          if (this.acState(input).open) this.acPosition(input, this.acList(input));
        });
      }, true);
    },
  };
};
