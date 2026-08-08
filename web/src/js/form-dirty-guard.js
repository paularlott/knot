// Global "unsaved changes" guard for create/edit form modals.
//
// A form opts in by adding `data-dirty-form` to its modal panel element
// (the `.ui-modal-panel*`). The guard then:
//   • marks the panel dirty when the user edits any field inside it (delegated
//     input/change listeners), including Ace code editors (a global Ace.edit
//     patch dispatches a `mark-dirty` event on each edit);
//   • intercepts Esc and the panel's close button while the panel is dirty,
//     showing a shared discard-confirmation dialog instead of closing;
//   • on confirm, clears the dirty flag and closes the panel by clicking its
//     own close button (so it uses whatever close logic that form already has);
//   • clears the dirty flag whenever a guarded panel is hidden (e.g. after a
//     successful save), so reopening starts clean.
//
// Template create/edit already has its own dirty/discard flow, so it is NOT
// tagged and is left untouched.

import Alpine from 'alpinejs';
import ace from 'ace-builds/src-noconflict/ace';

const DIRTY_ATTR = 'data-dirty-form';
const dirtyPanels = new WeakSet();

function panelOf(el) {
  return el && el.closest ? el.closest('[' + DIRTY_ATTR + ']') : null;
}

function isPanelVisible(panel) {
  return panel && window.getComputedStyle(panel).display !== 'none';
}

function markDirty(panel) {
  if (!panel) return;
  dirtyPanels.add(panel);
  panel.setAttribute('data-dirty', 'true');
}

function clearDirty(panel) {
  if (!panel) return;
  dirtyPanels.delete(panel);
  panel.removeAttribute('data-dirty');
}

// Topmost (last in DOM order) visible dirty form panel.
function topmostDirtyPanel() {
  let result = null;
  document
    .querySelectorAll('[' + DIRTY_ATTR + '][data-dirty="true"]')
    .forEach((p) => {
      if (isPanelVisible(p)) result = p;
    });
  return result;
}

let bypass = false;

// Close a panel by triggering its existing close button — works regardless of
// which component/state object drives the modal's visibility.
function closePanel(panel) {
  const btn = panel.querySelector('.ui-modal-close');
  if (!btn) return;
  bypass = true;
  btn.click();
  bypass = false;
}

function askDiscard(panel) {
  Alpine.store('discardDialog').ask(() => {
    clearDirty(panel);
    closePanel(panel);
  });
}

// Shared discard-confirmation dialog state.
Alpine.store('discardDialog', {
  show: false,
  _onConfirm: null,
  ask(onConfirm) {
    this._onConfirm = onConfirm;
    this.show = true;
  },
  confirm() {
    const fn = this._onConfirm;
    this._onConfirm = null;
    this.show = false;
    if (fn) fn();
  },
  cancel() {
    this._onConfirm = null;
    this.show = false;
  },
});

// --- Dirty tracking -------------------------------------------------------

// Field edits via event delegation.
document.addEventListener('input', (e) => markDirty(panelOf(e.target)), true);
document.addEventListener('change', (e) => markDirty(panelOf(e.target)), true);
// Ace code edits (dispatched by the patched Ace.edit below).
document.addEventListener('mark-dirty', (e) => markDirty(panelOf(e.target)), true);
// Successful save — forms emit a success alert. Clears the dirty flag so a
// "save and keep editing" flow doesn't then prompt to discard.
document.addEventListener('show-alert', (e) => {
  if (e.detail && e.detail.type === 'success') clearDirty(panelOf(e.target));
}, true);

// Patch Ace.edit once so every code editor marks its form dirty on change.
//
// Programmatic content loads (every form calls `editor.session.setValue(...)`
// when initialising) also fire Ace's `change` event. Without suppression that
// would mark the panel dirty the instant it opens, prompting "discard
// changes?" even though the user touched nothing. We wrap the session's
// setValue so the `change` events it emits synchronously do NOT dispatch
// `mark-dirty`. Genuine user edits (typing, paste, etc.) never go through
// setValue, so they still mark the form dirty as expected.
if (ace && typeof ace.edit === 'function') {
  const origEdit = ace.edit;
  ace.edit = function (...args) {
    const editor = origEdit.apply(this, args);
    try {
      let suppressDirty = false;
      const session = editor.session;
      if (session && typeof session.setValue === 'function') {
        const origSetValue = session.setValue;
        session.setValue = function (...setValueArgs) {
          suppressDirty = true;
          try {
            return origSetValue.apply(this, setValueArgs);
          } finally {
            suppressDirty = false;
          }
        };
      }
      session.on('change', () => {
        if (suppressDirty || !editor.container) return;
        editor.container.dispatchEvent(
          new CustomEvent('mark-dirty', { bubbles: true }),
        );
      });
    } catch (_) {
      // ignore — dirty tracking is best-effort
    }
    return editor;
  };
}

// --- Close interception --------------------------------------------------

document.addEventListener(
  'keydown',
  (e) => {
    if (bypass || e.key !== 'Escape') return;
    // If the discard dialog is open, Esc cancels it (and must not also close
    // the dirty form behind it).
    if (Alpine.store('discardDialog').show) {
      e.preventDefault();
      e.stopImmediatePropagation();
      Alpine.store('discardDialog').cancel();
      return;
    }
    const panel = topmostDirtyPanel();
    if (!panel) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    askDiscard(panel);
  },
  true,
);

document.addEventListener(
  'click',
  (e) => {
    if (bypass || Alpine.store('discardDialog').show) return;
    const btn = e.target.closest ? e.target.closest('.ui-modal-close') : null;
    if (!btn) return;
    const panel = panelOf(btn);
    if (!panel || !dirtyPanels.has(panel)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    Alpine.store('discardDialog').ask(() => {
      clearDirty(panel);
      bypass = true;
      btn.click();
      bypass = false;
    });
  },
  true,
);

// --- Reset dirty when a guarded panel is hidden --------------------------
// Covers close-via-save, close-via-event, etc. — any path that hides the panel.
const observer = new MutationObserver(() => {
  document.querySelectorAll('[' + DIRTY_ATTR + ']').forEach((p) => {
    if (!isPanelVisible(p)) clearDirty(p);
  });
});
observer.observe(document.body, {
  attributes: true,
  attributeFilter: ['style', 'class', 'hidden'],
  subtree: true,
});
