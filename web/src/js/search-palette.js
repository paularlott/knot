// Spotlight-style global search palette.
//
// Opens on Shift+Cmd/Ctrl+K (the unshifted Cmd/Ctrl+K stays bound to focusing
// the current page's own search box). Disabled on touch-primary devices.
//
// Searches server-side via GET /api/search (permission/zone/ownership-scoped,
// grouped, capped). Selecting a result navigates to that entity's list page
// with ?q=<name>, and a small bootstrap below seeds the list page's #search
// input from that param so the targeted entity is filtered into view.

import Alpine from 'alpinejs';

// Display order + destination page for each result group. Keys mirror the
// apiclient.SearchResults JSON fields.
const SEARCH_GROUPS = [
  { key: 'pages',      label: 'Pages',          url: '' },
  { key: 'spaces',      label: 'Spaces',          url: '/spaces' },
  { key: 'templates',   label: 'Templates',       url: '/templates' },
  { key: 'variables',   label: 'Variables',       url: '/variables' },
  { key: 'volumes',     label: 'Volumes',         url: '/volumes' },
  { key: 'stacks',      label: 'Stack Templates', url: '/stacks' },
  { key: 'scripts',     label: 'Scripts',         url: '/scripts' },
  { key: 'skills',      label: 'Skills',          url: '/skills' },
  { key: 'commands',    label: 'Slash Commands',  url: '/commands' },
  { key: 'mcp_servers', label: 'MCP Servers',     url: '/mcp-servers' },
  { key: 'event_sinks', label: 'Events',          url: '/events' },
  { key: 'users',       label: 'Users',           url: '/users' },
  { key: 'groups',      label: 'Groups',          url: '/groups' },
  { key: 'roles',       label: 'Roles',           url: '/roles' },
  { key: 'tokens',      label: 'API Tokens',      url: '/api-tokens' },
];

function isCoarse() {
  return window.matchMedia && window.matchMedia('(pointer: coarse)').matches;
}

// Matches the form-modal panel classes that movable-modal.js enhances. If any
// is visible, a form/dialog is open and the global search is blocked so it
// can't steal focus or keyboard from the open form.
const MODAL_SELECTOR = '.ui-modal-panel, .ui-modal-panel-wide, .ui-modal-panel-xl, .ui-modal-panel-2xl';
function isModalOpen() {
  return Array.from(document.querySelectorAll(MODAL_SELECTOR)).some(
    (el) => window.getComputedStyle(el).display !== 'none'
  );
}

Alpine.data('searchPalette', () => ({
  open: false,
  query: '',
  results: {},
  loading: false,
  activeIndex: -1,
  flat: [], // [{ type, hit }] across all groups, in display order
  reqId: 0,

  init() {
    // Global shortcut: Shift + Cmd/Ctrl + K. Ignored on touch-primary inputs.
    document.addEventListener('keydown', (e) => {
      if ((e.metaKey || e.ctrlKey) && e.shiftKey && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        this.openPalette();
      }
    });
    window.addEventListener('open-search-palette', () => this.openPalette());
  },

  get groupDefs() {
    return SEARCH_GROUPS;
  },

  hitsFor(key) {
    return this.results[key] || [];
  },

  get totalResults() {
    return this.flat.length;
  },

  // Key of the first group (in display order) that has matches, so the template
  // can render a divider above every other visible group but never above the
  // first — no stray line under the search input, even when earlier group types
  // have no matches for the query.
  get firstVisibleGroupKey() {
    for (const g of SEARCH_GROUPS) {
      if ((this.results[g.key] || []).length > 0) return g.key;
    }
    return null;
  },

  openPalette() {
    if (isCoarse() || isModalOpen()) return; // disabled on mobile/touch and while a form is open
    this.open = true;
    this.query = '';
    this.results = {};
    this.flat = [];
    this.activeIndex = -1;
    this.loading = false;
    this.$nextTick(() => this.$refs.input && this.$refs.input.focus());
  },

  close() {
    this.open = false;
  },

  async search() {
    const q = this.query.trim();
    if (!q) {
      this.results = {};
      this.flat = [];
      this.activeIndex = -1;
      this.loading = false;
      return;
    }
    this.loading = true;
    const id = ++this.reqId;
    try {
      const resp = await fetch('/api/search?q=' + encodeURIComponent(q), {
        headers: { Accept: 'application/json' },
      });
      if (id !== this.reqId) return; // a newer keystroke superseded this one
      if (!resp.ok) return;
      const data = await resp.json();
      if (id !== this.reqId) return;
      this.results = data || {};
      this.rebuildFlat();
      this.activeIndex = this.flat.length > 0 ? 0 : -1;
    } catch (_) {
      if (id === this.reqId) this.results = {};
    } finally {
      if (id === this.reqId) this.loading = false;
    }
  },

  rebuildFlat() {
    const flat = [];
    for (const g of SEARCH_GROUPS) {
      const hits = this.results[g.key];
      if (hits && hits.length) {
        for (const hit of hits) flat.push({ type: g.key, hit });
      }
    }
    this.flat = flat;
  },

  flatIndexFor(type, hit) {
    return this.flat.findIndex((f) => f.type === type && f.hit.id === hit.id);
  },

  moveDown() {
    if (this.flat.length === 0) return;
    this.activeIndex = (this.activeIndex + 1) % this.flat.length;
    this.scrollActiveIntoView();
  },

  moveUp() {
    if (this.flat.length === 0) return;
    this.activeIndex = (this.activeIndex - 1 + this.flat.length) % this.flat.length;
    this.scrollActiveIntoView();
  },

  scrollActiveIntoView() {
    this.$nextTick(() => {
      const el = this.$refs.list && this.$refs.list.querySelector('[data-idx="' + this.activeIndex + '"]');
      if (el) el.scrollIntoView({ block: 'nearest' });
    });
  },

  activateSelected() {
    if (this.activeIndex >= 0 && this.activeIndex < this.flat.length) {
      this.activate(this.flat[this.activeIndex]);
    }
  },

  activate(item) {
    if (!item) return;
    // Pages are navigation destinations — just open the page.
    if (item.type === 'pages') {
      window.location.href = item.hit.id || '/';
      return;
    }
    const g = SEARCH_GROUPS.find((x) => x.key === item.type);
    if (!g) return;
    const name = item.hit.name || '';
    const params = new URLSearchParams();
    if (item.type === 'spaces') {
      // Spaces land on their filtered list.
      if (name) params.set('q', name);
    } else {
      // Everything else opens straight in edit mode, and also seeds the page's
      // local search so the list is filtered to the item (e.g. when the edit
      // modal is closed the matched row is still in view).
      if (item.hit.id) params.set('edit', item.hit.id);
      if (name) params.set('q', name);
    }
    const qs = params.toString();
    window.location.href = g.url + (qs ? '?' + qs : '');
  },
}));

// If a list page was opened with ?q=<term> (e.g. from the palette), seed the
// page's #search input — present on every list page, bound via x-model — so the
// targeted entity is filtered into view. Runs once after Alpine is up so the
// list components exist; they re-apply their filter when their data loads.
function prefillPageSearchFromQuery() {
  const q = new URLSearchParams(window.location.search).get('q');
  if (!q) return;
  const input = document.getElementById('search');
  if (!input) return;
  input.value = q;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  // Strip ?q= so a refresh or shared link doesn't re-trigger.
  const clean = new URL(window.location.href);
  clean.searchParams.delete('q');
  window.history.replaceState(null, '', clean.toString());
}

document.addEventListener('alpine:initialized', prefillPageSearchFromQuery);

// Cmd/Ctrl+K (without Shift) focuses the current page's #search filter input
// when one is present — i.e. on list pages. Global so every list page gets the
// local-search shortcut without per-component wiring. Shift+Cmd/Ctrl+K opens
// the global palette (handled in the Alpine component above).
document.addEventListener('keydown', (e) => {
  if ((e.metaKey || e.ctrlKey) && !e.shiftKey && (e.key === 'k' || e.key === 'K')) {
    const input = document.getElementById('search');
    if (input && !isModalOpen()) {
      e.preventDefault();
      input.focus();
      if (typeof input.select === 'function') input.select();
    }
  }
});
