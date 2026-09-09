// Plugin page runtime: the handler returns a LAYOUT - rows of columns,
// each column declaring its type, data handler and refresh. This component
// renders the shell (row grid, column cards, loaders), fetches each
// column's data from its handler URL (the page path plus /<handler>), and
// renders it with the block renderers from pluginBlocks.js. Form columns
// POST to their own handler URL; the response envelope (status/message/
// field_errors/refresh/dialog) drives notifications, column refreshes and
// information dialogs. Table actions render as icon buttons, text chips or
// kebab-menu items; actions naming a handler open a popup (form or
// markdown) in a knot-styled modal. All wiring is delegated from the page
// root so patched columns never re-bind.

import { renderBlock, initPluginChart, resetPluginForm, chromeIcon } from './pluginBlocks.js';

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

// Column spans track the grid density: rows are grid-cols-1 / sm:grid-cols-2
// / xl:grid-cols-4, so a column only claims its declared width once the row
// actually has four tracks. An unconditional span wider than the grid would
// spawn implicit zero-width tracks and squeeze the columns after it.
const SPAN = {
  1: 'col-span-1',
  2: 'col-span-1 sm:col-span-2',
  3: 'col-span-1 sm:col-span-2 xl:col-span-3',
  4: 'col-span-1 sm:col-span-2 xl:col-span-4',
};

// assetIcon builds the inline <svg> for a plugin's declared asset icon:
// sanitized markup shipped with the page document's icons map, rendered
// outline-styled (currentColor) like every plugin icon. An icon the plugin
// did not declare renders no icon at all (a text button) — the map is the
// boundary. Knot's own runtime icons (dialog headers, kebab, close) are
// chromeIcon clones of the pb-icon-* template fragments instead.
function assetIcon(markup, className = 'size-5') {
  if (!markup) return null;
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('xmlns', 'http://www.w3.org/2000/svg');
  svg.setAttribute('viewBox', '0 0 24 24');
  svg.setAttribute('class', className);
  svg.setAttribute('aria-hidden', 'true');
  svg.setAttribute('fill', 'none');
  svg.setAttribute('stroke', 'currentColor');
  svg.setAttribute('stroke-width', '1.5');
  svg.innerHTML = markup;
  return svg;
}

// Text-chip action buttons use the spaces table colour semantics.
// knot's auth middleware answers 503 on transient store hiccups
// specifically so clients retry instead of failing; honour that.
async function fetchWithRetry(url, options, attempts = 3) {
  let delay = 300;
  for (let i = 0; ; i += 1) {
    const response = await fetch(url, options);
    if (response.status !== 503 || i >= attempts - 1) return response;
    await new Promise((r) => setTimeout(r, delay));
    delay *= 3;
  }
}

function actionChipClass(style) {
  const colors = {
    danger: 'text-red-700 dark:text-red-400',
    success: 'text-green-700 dark:text-green-400',
    warning: 'text-amber-700 dark:text-amber-300',
  };
  return 'inline-flex items-center gap-1.5 rounded-lg border border-slate-300 bg-white px-2.5 py-1.5 text-xs font-medium transition-colors hover:bg-slate-100 mr-1 dark:border-slate-700 dark:bg-slate-800/45 dark:hover:bg-slate-700/70 '
    + (colors[style] || 'text-blue-700 dark:text-blue-400');
}

window.pluginPage = function pluginPage(url) {
  // Handler URLs are paths: /plugins/<name>/<page-path>/<handler> runs the
  // handler through that page's gate, /plugins/<name>/<handler> through the
  // plugin's default page. Handlers are ajax endpoints — any page (this
  // plugin's or another's) may fetch them; the gate is always the requesting
  // user's permission on the page the URL rides.
  const pluginName = url.split('/')[2] || '';

  // handlerURL builds the fetch URL for a handler: same-plugin handlers are
  // relative to the page URL (no plugin qualifier needed), any other plugin
  // is addressed at its root.
  const handlerURL = (handler, plugin) => (plugin && plugin !== pluginName
    ? `/plugins/${encodeURIComponent(plugin)}/${encodeURIComponent(handler)}`
    : `${url}/${encodeURIComponent(handler)}`);

  // pluginFetch is the trusted-html API for calling plugin handlers from
  // client-side code (Alpine widgets and the like): the same transport,
  // auth, page gate and running-user identity as every column fetch.
  //   const data = await pluginFetch('my_handler');
  //   await pluginFetch('my_handler', { method: 'POST', body: {name: 'x'} });
  //   await pluginFetch('echo', { plugin: 'demo-go' }); // another plugin's handler
  window.pluginFetch = async function pluginFetch(handler, options = {}) {
    const qs = new URLSearchParams(options.params || {});
    const request = options.method === 'POST'
      ? {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: new URLSearchParams(options.body || {}).toString(),
      }
      : { headers: { 'Content-Type': 'application/json' } };
    const target = handlerURL(handler, options.plugin);
    const response = await fetchWithRetry(qs.toString() ? `${target}?${qs}` : target, request);
    const text = await response.text();
    try {
      return JSON.parse(text);
    } catch (e) {
      throw new Error('plugin handler returned a non-JSON response (session or server?)');
    }
  };

  return {
    url,
    params: '',
    timers: {},
    _timerSeq: 0,
    _ac: new WeakMap(),

    handlerURL,

    init() {
      const embedded = document.getElementById('plugin-document');
      const region = this.region();
      if (!embedded || !region) return;
      this.params = new URLSearchParams(window.location.search).toString();
      let doc = null;
      try { doc = JSON.parse(embedded.textContent); } catch (e) { /* bad doc */ }
      // The document may ship the plugin's declared icon assets (path →
      // sanitized inner SVG, loaded at plugin load) for data-driven action
      // icons that name a path instead of a built-in.
      this.icons = (doc && doc.icons) || {};
      if (doc) this.render(doc);
      this.bindRoot(region);
      // Kebab menus close on any click outside them (the trigger stops
      // propagation) and on Escape. Modal/autocomplete Escapes stop
      // propagation first, so they never reach here.
      document.addEventListener('click', () => this.closeMenus());
      document.addEventListener('keydown', (event) => {
        if (event.key === 'Escape') this.closeMenus();
      });
    },

    region() {
      return this.$root.querySelector('[data-plugin-region]');
    },

    // ---- layout ------------------------------------------------------

    render(doc) {
      const region = this.region();
      if (!region) return;
      // A full re-render wipes the region: any open popup must release its
      // scroll lock and focus through the normal close path first.
      this.closeModal();
      this.closeMenus();
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
      wrap.dataset.colType = column.type || '';
      if (column.title) wrap.appendChild(el('h3', 'text-sm font-semibold text-gray-900 dark:text-white break-words', column.title));
      const body = el('div', 'space-y-3 flex-1 flex flex-col');
      body.dataset.colBody = '1';
      wrap.appendChild(body);
      this.showLoader(body);
      if (column.handler) this.fetchColumn(column, body);
      else this.renderColumnData(column, {}, body);
      if (column.refresh) {
        // Stagger refresh timers across the period so a page's columns
        // don't all fire in the same tick: five slots spread the load
        // server-side instead of bursting it with every refresh.
        const period = column.refresh * 1000;
        const slot = this._timerSeq = (this._timerSeq + 1) % 5;
        this.timers[column.id] = setTimeout(() => {
          this.timers[column.id] = setInterval(() => {
            if (!this.userReading()) this.fetchColumn(column, body, true);
          }, period);
        }, (period / 5) * slot);
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



    fetchWithRetry(url, options, attempts) {
      return fetchWithRetry(url, options, attempts);
    },

    async fetchColumn(column, body, isRefresh) {
      body.closest('[data-col-id]')?.setAttribute('aria-busy', 'true');
      try {
        const target = this.handlerURL(column.handler);
        const response = await this.fetchWithRetry(this.params ? `${target}?${this.params}` : target, { headers: { 'Content-Type': 'application/json' } });
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
      const rows = data.rows || [];
      const columnActions = Array.isArray(column.actions) ? column.actions : [];
      const anyRowActions = rows.some((row) => row && Array.isArray(row.actions));
      if (!columnActions.length && !anyRowActions) return tableBlock;
      const head = tableBlock.querySelector('[data-head]');
      if (head && head.rows.length) {
        const th = document.createElement('th');
        th.scope = 'col';
        th.className = 'px-4 py-3';
        head.rows[0].appendChild(th);
      }
      const bodyEl = tableBlock.querySelector('[data-body]');
      [...bodyEl.rows].forEach((tr, i) => {
        const row = rows[i] || {};
        // A row's own actions list (even empty) replaces the column's set:
        // the handler decides per row what is offered.
        const actions = Array.isArray(row.actions) ? row.actions : columnActions;
        if (!actions.length) return;
        const td = tr.insertCell(-1);
        td.className = 'px-4 py-3 whitespace-nowrap';
        td.appendChild(this.renderActionSet(actions, String(row.id || row.name || '')));
      });
      return tableBlock;
    },

    // ---- row actions: icon buttons, chips and the kebab menu ------------

    renderActionSet(actions, key) {
      const host = el('div', 'flex items-center justify-end gap-1');
      const menuActions = actions.filter((action) => action.menu);
      actions.filter((action) => !action.menu).forEach((action) => host.appendChild(this.actionButton(action, key)));
      if (menuActions.length) host.appendChild(this.menuTrigger(menuActions, key));
      return host;
    },

    // An action icon names a declared icon asset (path); its sanitized
    // markup ships with the page document (this.icons). Anything else —
    // unknown name, undeclared path — renders a text button.
    actionIcon(name, className = 'size-5') {
      return assetIcon(name && this.icons ? this.icons[name] : null, className);
    },

    actionButton(action, key) {
      let btn;
      const icon = this.actionIcon(action.icon, 'size-5');
      if (icon) {
        // Icon-only buttons match the spaces list rows: the label rides in
        // the title tooltip and a screen-reader-only span.
        btn = el('button', `row-action-button${action.style ? ` row-action-${action.style}` : ''}`);
        btn.appendChild(icon);
        btn.appendChild(el('span', 'sr-only', action.label));
        btn.title = action.label;
        btn.setAttribute('aria-label', action.label);
      } else {
        btn = el('button', actionChipClass(action.style), action.label);
      }
      this.armAction(btn, action, key);
      return btn;
    },

    armAction(btn, action, key) {
      btn.type = 'button';
      btn.dataset.action = action.action || '';
      btn.dataset.key = key;
      if (action.confirm) btn.dataset.confirm = action.confirm;
      if (action.style) btn.dataset.actionStyle = action.style;
      if (action.handler) btn.dataset.popup = action.handler;
    },

    menuTrigger(menuActions, key) {
      const host = el('div', 'relative');
      const btn = el('button', 'row-action-menu-trigger');
      btn.type = 'button';
      btn.setAttribute('aria-haspopup', 'menu');
      btn.setAttribute('aria-expanded', 'false');
      btn.setAttribute('aria-label', 'More actions');
      btn.appendChild(chromeIcon('kebab', 'size-5'));
      const panel = el('div', 'fixed z-50 my-1 p-2 bg-white rounded-lg shadow-xl border border-gray-200 dark:bg-gray-800 dark:border-gray-700 whitespace-nowrap');
      panel.setAttribute('role', 'menu');
      panel.style.display = 'none';
      panel.dataset.pluginMenu = '1';
      panel._trigger = btn;
      menuActions.forEach((action) => {
        const item = el('button', `nav-item text-sm px-4 w-full${action.style === 'danger' ? ' text-red-700 hover:bg-red-50 hover:text-red-800 dark:text-red-400 dark:hover:bg-red-900/30 dark:hover:text-red-300' : ''}`);
        item.type = 'button';
        item.setAttribute('role', 'menuitem');
        const icon = this.actionIcon(action.icon, 'size-4 mr-2');
        if (icon) item.appendChild(icon);
        item.appendChild(document.createTextNode(action.label));
        this.armAction(item, action, key);
        panel.appendChild(item);
      });
      btn.addEventListener('click', (event) => {
        event.stopPropagation();
        this.toggleMenu(panel);
      });
      host.appendChild(btn);
      host.appendChild(panel);
      return host;
    },

    toggleMenu(panel) {
      const wasOpen = panel.style.display !== 'none';
      this.closeMenus();
      if (wasOpen) return;
      panel.style.display = 'block';
      if (panel._trigger) panel._trigger.setAttribute('aria-expanded', 'true');
      // Anchor to the trigger's bottom-right corner, flipping up when the
      // viewport is cramped below (same approach as the autocompleter).
      const rect = panel._trigger ? panel._trigger.getBoundingClientRect() : { right: 0, bottom: 0, top: 0 };
      const pw = panel.offsetWidth;
      const ph = panel.offsetHeight;
      const left = Math.max(8, Math.min(rect.right - pw, window.innerWidth - pw - 8));
      const below = window.innerHeight - rect.bottom;
      // Prefer the roomier side; when neither fully fits (trigger at the
      // viewport edge), clamp so the menu always stays on screen.
      const top = below >= ph + 8
        ? rect.bottom + 4
        : Math.max(8, Math.min(rect.top - ph - 4, window.innerHeight - ph - 8));
      panel.style.left = `${Math.round(left)}px`;
      panel.style.top = `${Math.round(top)}px`;
    },

    closeMenus() {
      this.region()?.querySelectorAll('[data-plugin-menu]').forEach((panel) => {
        panel.style.display = 'none';
        if (panel._trigger) panel._trigger.setAttribute('aria-expanded', 'false');
      });
    },

    // ---- popups: knot's ui-modal look, imperative lifetime ---------------
    // Panels built with the ui-modal-* classes are picked up by
    // movable-modal.js, so they are draggable and resizable like knot's own
    // dialogs. Modals live inside the plugin region so the delegated submit
    // and autocompleter wiring keeps working without rebinding.

    openModal({ title, danger = false }) {
      this.closeModal();
      const root = el('div', 'ui-modal-backdrop');
      root.dataset.pluginModal = '1';
      root.setAttribute('role', 'dialog');
      root.setAttribute('aria-modal', 'true');
      root.setAttribute('aria-label', title);
      const panel = el('div', 'ui-modal-panel rounded-2xl max-h-[90vh]');
      const header = el('div', 'ui-modal-header rounded-t-2xl');
      const iconDiv = el('div', danger ? 'ui-modal-icon-danger' : 'ui-modal-icon-info');
      iconDiv.appendChild(chromeIcon(danger ? 'trash' : 'info', 'size-6'));
      const heading = el('h3', 'ui-modal-title', title);
      const closeBtn = el('button', 'ui-modal-close');
      closeBtn.type = 'button';
      closeBtn.setAttribute('aria-label', 'Close');
      closeBtn.appendChild(chromeIcon('close', 'size-5'));
      header.append(iconDiv, heading, closeBtn);
      const body = el('div', 'ui-modal-body-scroll');
      panel.append(header, body);
      root.appendChild(panel);
      this.region().appendChild(root);
      const trap = (event) => {
        if (event.key === 'Escape') {
          event.stopPropagation();
          this.closeModal();
          return;
        }
        if (event.key !== 'Tab') return;
        const focusables = panel.querySelectorAll('button, input, select, textarea, a[href], [tabindex]:not([tabindex="-1"])');
        if (!focusables.length) return;
        const first = focusables[0];
        const last = focusables[focusables.length - 1];
        if (event.shiftKey && document.activeElement === first) {
          event.preventDefault();
          last.focus();
        } else if (!event.shiftKey && document.activeElement === last) {
          event.preventDefault();
          first.focus();
        }
      };
      // The trap lives on the document while the modal is open (knot's own
      // dialogs use @keydown.esc.window): after a click on the backdrop the
      // focus sits outside the panel, and a modal-scoped listener would be
      // dead exactly then. Autocompleter Escape stopPropagation keeps an
      // open suggestion list from closing the modal underneath it.
      document.addEventListener('keydown', trap);
      closeBtn.addEventListener('click', () => this.closeModal());
      const prevOverflow = document.body.style.overflow;
      document.body.style.overflow = 'hidden';
      this._modal = {
        root,
        panel,
        body,
        heading,
        // The dialog opens titled "Loading …"; keep aria-label in step when
        // the real title arrives.
        setTitle(text) {
          heading.textContent = text;
          root.setAttribute('aria-label', text);
        },
        restore: document.activeElement,
        prevOverflow,
        trap,
      };
      requestAnimationFrame(() => {
        const firstControl = panel.querySelector('input, select, textarea, button:not(.ui-modal-close)');
        (firstControl || closeBtn).focus({ preventScroll: true });
      });
      return this._modal;
    },

    closeModal() {
      if (!this._modal) return;
      const modal = this._modal;
      this._modal = null;
      document.removeEventListener('keydown', modal.trap);
      document.body.style.overflow = modal.prevOverflow;
      modal.root.remove();
      if (modal.restore && modal.restore.isConnected) modal.restore.focus({ preventScroll: true });
    },

    modalFooter(modal, buttons) {
      const footer = el('div', 'ui-modal-footer');
      buttons.forEach(({ label, danger, icon, onClick }) => {
        const btn = el('button', danger ? 'ui-button-danger' : 'ui-button-secondary');
        btn.type = 'button';
        // An icon'd action button matches knot's own dialogs (the group
        // delete's trash-icon'd Delete Group): icon left, label right.
        // icon names a pb-icon-* chrome fragment.
        if (icon) btn.appendChild(chromeIcon(icon, 'size-4 mr-2'));
        btn.appendChild(document.createTextNode(label));
        btn.addEventListener('click', onClick);
        footer.appendChild(btn);
      });
      modal.panel.appendChild(footer);
      return footer;
    },

    // An action with a handler GETs it on click; the response shape decides
    // what the popup is: fields -> form, html/markdown -> information view.
    async openPopup(handler, key, trigger) {
      const modal = this.openModal({ title: 'Loading …' });
      this.showLoader(modal.body);
      try {
        const target = `${this.handlerURL(handler)}?key=${encodeURIComponent(key)}`;
        const response = await this.fetchWithRetry(target, { headers: { 'Content-Type': 'application/json' } });
        if (response.status !== 200) throw new Error(`HTTP ${response.status}`);
        const data = await response.json();
        if (data && data.error) throw new Error(data.error);
        if (data && Array.isArray(data.fields)) {
          this.buildPopupForm(modal, handler, key, data);
        } else if (data && typeof data.html === 'string') {
          modal.setTitle(data.title || 'Details');
          const content = el('div', 'pb-markdown text-sm text-gray-700 dark:text-gray-300');
          content.innerHTML = data.html; // trusted: server-rendered from plugin markdown
          modal.body.innerHTML = '';
          modal.body.appendChild(content);
          this.modalFooter(modal, [{ label: 'Close', onClick: () => this.closeModal() }]);
        } else {
          throw new Error('unexpected popup response');
        }
      } catch (e) {
        if (!this._modal || this._modal.root !== modal.root) return; // replaced or closed
        modal.body.innerHTML = '';
        modal.body.appendChild(el('div', 'rounded-lg border border-red-200 dark:border-red-900 p-3 text-sm text-red-700 dark:text-red-300', 'This action failed to load.'));
        this.modalFooter(modal, [{ label: 'Close', onClick: () => this.closeModal() }]);
      }
    },

    buildPopupForm(modal, handler, key, data) {
      modal.setTitle(data.title || 'Edit');
      // Form popups join the shared dirty guard (form-dirty-guard.js):
      // field edits mark the panel, and Esc / the close button confirm
      // before discarding, exactly like knot's own form modals. A
      // successful submit closes the popup, and the guard clears the
      // flag when the panel hides, so reopening starts clean.
      modal.panel.setAttribute('data-dirty-form', '');
      const form = renderBlock({
        type: 'form',
        fields: data.fields,
        submit: data.submit,
        cancel: data.cancel || 'Cancel',
      }, {
        // Dynamic options come from the popup's own handler.
        onDynamicOptions: (input) => this.fetchDynamicOptions({ id: handler, handler }, input),
      });
      form.dataset.colId = handler;
      form.dataset.colHandler = handler;
      form.dataset.actionKey = key;
      modal.body.innerHTML = '';
      modal.body.appendChild(form);
      // Knot dialogs right-align their actions in a modal footer; the
      // buttons keep working from there through explicit form association.
      form.id = 'plugin-popup-form';
      const footer = el('div', 'ui-modal-footer');
      const submit = form.querySelector('[data-submit]');
      const cancel = form.querySelector('[data-cancel]');
      // knot's form dialogs put the secondary action left (sm:mr-auto) and
      // the primary submit right.
      if (cancel) {
        cancel.className = 'ui-button-secondary sm:mr-auto';
        cancel.setAttribute('form', form.id);
        footer.appendChild(cancel);
      }
      if (submit) {
        submit.setAttribute('form', form.id);
        footer.appendChild(submit);
      }
      modal.panel.appendChild(footer);
    },

    // A success envelope may open an information dialog (markdown rendered
    // server-side) alongside the toast.
    openDialog(dialog) {
      const modal = this.openModal({ title: dialog.title || 'Information' });
      const content = el('div', 'pb-markdown text-sm text-gray-700 dark:text-gray-300');
      content.innerHTML = dialog.html || ''; // trusted: server-rendered from plugin markdown
      modal.body.appendChild(content);
      this.modalFooter(modal, [{ label: 'Close', onClick: () => this.closeModal() }]);
    },

    // A row action's confirm dialog, mirroring knot's own delete dialogs
    // (the group delete): the action supplies the copy, the style picks the
    // treatment — a danger action gets the delete look (trash header icon,
    // Keep, a trash-icon'd confirm button carrying the action label),
    // anything else the neutral confirm (info header, Cancel / Confirm).
    confirmModal({ message, confirmLabel, danger = false, run }) {
      const modal = this.openModal({ title: danger ? 'Confirm Delete' : 'Confirm', danger });
      modal.body.appendChild(el('p', 'text-center', message));
      this.modalFooter(modal, [
        { label: danger ? 'Keep' : 'Cancel', onClick: () => this.closeModal() },
        { label: confirmLabel || 'Confirm', danger, icon: danger ? 'trash' : null, onClick: () => { this.closeModal(); run(); } },
      ]);
    },

    // An action's effect settles over the next seconds (a start goes
    // pending -> running). Re-fetch the refreshed columns a few times so
    // they converge. Unlike the idle timers this does not defer for
    // hover/focus: the user just acted, the pointer is on the table, and
    // the patches are loader-free in-place updates.
    scheduleRefreshBurst(targets) {
      [2000, 5000, 9000, 15000].forEach((delay) => {
        setTimeout(() => {
          if (!this.region()) return;
          this.region().querySelectorAll('[data-col-id]').forEach((node) => {
            if (targets && !targets.includes(node.dataset.colId)) return;
            const column = { id: node.dataset.colId, type: node.dataset.colType, handler: node.dataset.colHandler };
            const body = node.querySelector('[data-col-body]');
            if (column.handler) this.fetchColumn(column, body, true);
          });
        }, delay);
      });
    },

    runAction(btn) {
      const columnId = btn.closest('[data-col-handler]')?.dataset.colHandler || '';
      const body = new URLSearchParams({ action: btn.dataset.action, key: btn.dataset.key }).toString();
      btn.disabled = true;
      this.fetchWithRetry(this.handlerURL(columnId), {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body,
      }).then((r) => r.json()).then((result) => {
        btn.disabled = false;
        this.handleEnvelope(null, result);
      }).catch(() => { btn.disabled = false; this.toast('The action failed to submit.', 'error'); });
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
        const target = `${this.handlerURL(column.handler)}?_data=${encodeURIComponent(input.name)}`;
        const data = await this.fetchWithRetry(target, { headers: { 'Content-Type': 'application/json' } }).then((r) => r.json());
        let options = Array.isArray(data) ? data : data && data.options;
        if (!Array.isArray(options)) options = [];
        input.dataset.pbOptions = JSON.stringify(options.map((o) => (typeof o === 'object' && o !== null ? { key: o.key, text: o.text || o.key } : { key: String(o), text: String(o) })));
      } catch (e) { /* keep whatever fixed options the field declared */
      }
    },

    // ---- forms: POST to the column endpoint, handle the envelope ------

    async submitForm(form) {
      const columnId = form.dataset.colHandler || form.closest('[data-col-handler]')?.dataset.colHandler;
      // Popup forms submit with their row key so the handler knows what to act on.
      const target = form.dataset.actionKey
        ? `${this.handlerURL(columnId)}?key=${encodeURIComponent(form.dataset.actionKey)}`
        : this.handlerURL(columnId);
      // Clear stale field errors before submitting.
      form.querySelectorAll('.form-field-error').forEach((node) => node.classList.remove('form-field-error'));
      form.querySelectorAll('.error-message').forEach((node) => node.remove());
      const submitBtn = form.querySelector('[type="submit"]');
      if (submitBtn) submitBtn.disabled = true;
      const wrap = form.closest('[data-col-id]') || form.closest('[data-plugin-modal]') || form.parentElement;
      wrap?.setAttribute('aria-busy', 'true');
      try {
        const body = new URLSearchParams(new FormData(form)).toString();
        const response = await this.fetchWithRetry(target, {
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
        if (submitBtn) submitBtn.disabled = false;
        wrap?.setAttribute('aria-busy', 'false');
      }
    },

    handleEnvelope(form, envelope) {
      if (!envelope || envelope.status === undefined) return;
      if (envelope.status === 'ok') {
        this.toast(envelope.message || 'Done.', 'ok');
        // Popup forms: success closes the popup. Filter forms instead fold
        // their fields into the page's params so refreshed columns fetch
        // with the new filters; plain forms keep the params fold (harmless
        // for one-shot forms) and then reset, so the next entry starts
        // clean — one note after another without retyping-over.
        const popup = form && form.closest('[data-plugin-modal]');
        if (popup) {
          this.closeModal();
        } else if (form) {
          try { this.params = new URLSearchParams(new FormData(form)).toString(); } catch (e) { /* form detached */ }
          if (!form.dataset.pluginAuto) resetPluginForm(form);
        }
        // Refresh the columns the handler names (or all). Each node drives
        // its own handler (declared on the wrapper), so duplicate column
        // ids across rows can never cross-wire a refresh.
        const targets = envelope.refresh === true ? null : envelope.refresh;
        if (envelope.refresh) {
          this.region().querySelectorAll('[data-col-id]').forEach((node) => {
            if (!targets || targets.includes(node.dataset.colId)) {
              const body = node.querySelector('[data-col-body]');
              const column = { id: node.dataset.colId, type: node.dataset.colType, handler: node.dataset.colHandler };
              if (column.handler) { this.showLoader(body); this.fetchColumn(column, body, true); }
            }
          });
          this.scheduleRefreshBurst(targets);
        }
        if (envelope.dialog) this.openDialog(envelope.dialog);
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
        // A popup form's cancel button closes its modal — routed through
        // the modal's close button so the shared dirty guard asks before
        // discarding when the form has edits. A clean panel sails through
        // the guard and the button's own listener closes the modal; a
        // dirty one gets the Keep Editing / Discard Changes dialog, the
        // same as Esc and the X button.
        const cancel = event.target.closest('[data-plugin-form-cancel]');
        if (cancel && cancel.closest('[data-plugin-modal]')) {
          const closeBtn = cancel.closest('.ui-modal-panel')?.querySelector('.ui-modal-close');
          if (closeBtn) {
            closeBtn.click();
            return;
          }
          this.closeModal();
          return;
        }
        const btn = event.target.closest('button[data-action]');
        if (!btn) return;
        this.closeMenus();
        if (btn.dataset.popup) {
          this.openPopup(btn.dataset.popup, btn.dataset.key, btn);
          return;
        }
        const run = () => this.runAction(btn);
        // Every confirm is the modal dialog: an inline arm cannot hold its
        // text on an icon button, and one consistent pattern beats two. The
        // action's style drives the dialog's treatment (delete look vs
        // neutral), carried on the button at arm time.
        if (btn.dataset.confirm) {
          this.confirmModal({
            message: btn.dataset.confirm,
            confirmLabel: btn.textContent.trim() || 'Confirm',
            danger: btn.dataset.actionStyle === 'danger',
            run,
          });
          return;
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
        this.closeMenus();
        region.querySelectorAll('[data-ac-input]').forEach((input) => {
          if (this.acState(input).open) this.acPosition(input, this.acList(input));
        });
      }, true);
    },
  };
};
