// Shared, accessible confirmation dialog and toast helpers.
//
//   knotConfirm({ title, message, name, detail, confirmLabel, cancelLabel,
//                 danger, icon, checkbox }) -> Promise<boolean>
//
// `message` may contain `{name}`; the `name` value is rendered in bold at that
// position (always as text — never HTML). `detail` is an optional extra
// paragraph (string, or an array of strings for several paragraphs).
// `checkbox: { label, checked }` adds a checkbox to the dialog; when supplied
// the promise resolves to `{ confirmed, checked }` instead of a boolean.
//
// `danger: true` gives the destructive look (red confirm button, trash icon,
// title "Confirm Delete", cancel "Keep"); otherwise a neutral look (title
// "Confirm", cancel "Cancel", info icon). `icon` overrides the icon:
// 'trash' | 'stop' | 'warning' | 'info'.
//
// Only one confirm is open at a time: opening another while one is showing
// resolves the first as cancelled.
//
// The markup lives in templates/partials/confirm-dialog.tmpl and is included
// from every layout.
//
//   knotToast(message, kind) — kind: 'success' | 'error' | 'info'
//
// Shows a message through knot's global alert stack (the `show-alert` window
// event handled by templates/partials/alerts.tmpl).

import Alpine from 'alpinejs';
import { recentDisclosureTrigger } from './disclosure.js';

const ICONS = ['trash', 'stop', 'warning', 'info'];

function isVisible(el) {
  return !!(el && el.isConnected && el.getClientRects().length > 0);
}

function splitMessage(message, name) {
  const text = message == null ? '' : String(message);
  if (name == null || name === '' || text.indexOf('{name}') === -1) {
    return [{ text, bold: false }];
  }
  const parts = [];
  text.split('{name}').forEach((piece, i) => {
    if (i > 0) parts.push({ text: String(name), bold: true });
    if (piece) parts.push({ text: piece, bold: false });
  });
  return parts;
}

// Work out where focus should return when the dialog closes. Items inside a
// row-action dropdown are hidden by the time the dialog closes, so fall back
// to the dropdown's trigger button.
function resolveOpener() {
  let el = document.activeElement;
  if (!el || el === document.body || el === document.documentElement) {
    return recentDisclosureTrigger();
  }
  const panel = el.closest ? el.closest('[data-knot-disclosure]') : null;
  if (panel && panel.trigger) return panel.trigger;
  return el;
}

function dialogRoot() {
  return document.getElementById('knot-confirm-dialog');
}

Alpine.store('confirmDialog', {
  show: false,
  title: '',
  parts: [],
  details: [],
  confirmLabel: '',
  cancelLabel: '',
  danger: false,
  icon: 'info',
  hasCheckbox: false,
  checkboxLabel: '',
  checked: false,
  _resolve: null,
  _withCheckbox: false,
  _opener: null,

  get iconClass() {
    if (this.icon === 'warning') return 'ui-modal-icon';
    if (this.icon === 'info') return 'ui-modal-icon-info';
    return this.danger ? 'ui-modal-icon-danger' : 'ui-modal-icon-info';
  },

  open(opts = {}) {
    // Only one at a time — a new request cancels the one showing.
    if (this._resolve) this._finish(false, true);

    const danger = !!opts.danger;
    this.danger = danger;
    this.title = opts.title || (danger ? 'Confirm Delete' : 'Confirm');
    this.parts = splitMessage(opts.message, opts.name);
    const detail = opts.detail;
    this.details = detail == null || detail === ''
      ? []
      : (Array.isArray(detail) ? detail : [detail]).map(String);
    this.confirmLabel = opts.confirmLabel || (danger ? 'Delete' : 'Confirm');
    this.cancelLabel = opts.cancelLabel || (danger ? 'Keep' : 'Cancel');
    this.icon = ICONS.includes(opts.icon) ? opts.icon : (danger ? 'trash' : 'info');
    this._withCheckbox = !!opts.checkbox;
    this.hasCheckbox = this._withCheckbox;
    this.checkboxLabel = this._withCheckbox ? String(opts.checkbox.label || '') : '';
    this.checked = this._withCheckbox ? !!opts.checkbox.checked : false;
    this._opener = resolveOpener();

    const promise = new Promise((resolve) => { this._resolve = resolve; });
    this.show = true;

    Alpine.nextTick(() => {
      const root = dialogRoot();
      if (!root) return;
      // An already-open modal's x-trap.inert marks its siblings (this dialog
      // included) aria-hidden; this dialog must stay exposed above it.
      root.removeAttribute('aria-hidden');
      const target = root.querySelector(danger ? '[data-confirm-cancel]' : '[data-confirm-ok]');
      if (target) target.focus();
      // The focus trap activates ~15ms after showing; make sure focus landed.
      setTimeout(() => {
        if (this.show && target && !root.contains(document.activeElement)) target.focus();
      }, 40);
    });

    return promise;
  },

  confirm() {
    this._finish(true);
  },

  cancel() {
    this._finish(false);
  },

  _finish(confirmed, replacing = false) {
    const resolve = this._resolve;
    const opener = this._opener;
    const withCheckbox = this._withCheckbox;
    const checked = this.checked;
    this._resolve = null;
    this._opener = null;
    if (!replacing) this.show = false;
    if (resolve) resolve(withCheckbox ? { confirmed, checked } : confirmed);
    if (!replacing && isVisible(opener)) {
      setTimeout(() => {
        // Don't steal focus from something the confirm action moved it to
        // (e.g. a dialog opened as a result).
        const active = document.activeElement;
        if (!active || active === document.body || !isVisible(active) || dialogRoot()?.contains(active)) {
          opener.focus({ preventScroll: true });
        }
      }, 0);
    }
  },
});

// Escape cancels the confirm and must not also close whatever modal is
// underneath it, so handle it in the capture phase on window (before any
// other Escape handler) and stop it there.
window.addEventListener(
  'keydown',
  (e) => {
    if (e.key !== 'Escape') return;
    const store = Alpine.store('confirmDialog');
    if (!store || !store.show) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    store.cancel();
  },
  true,
);

window.knotConfirm = function knotConfirm(opts) {
  return Alpine.store('confirmDialog').open(opts || {});
};

const TOAST_KINDS = { success: 'success', error: 'error', info: 'info' };

function escapeHtml(s) {
  return String(s == null ? '' : s)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}

window.knotToast = function knotToast(message, kind = 'success') {
  const type = TOAST_KINDS[kind] || 'info';
  window.dispatchEvent(new CustomEvent('show-alert', {
    detail: { type, msg: escapeHtml(message) },
  }));
};

export const knotConfirm = window.knotConfirm;
export const knotToast = window.knotToast;
