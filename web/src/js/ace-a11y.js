// Accessible names for Ace code editors.
//
// Ace replaces a <textarea> host with its own element and takes keyboard
// input through a hidden textarea of its own, so a <label for="..."> on the
// original host labels nothing once the editor is created. This patches
// Ace.edit once so every editor's input textarea is named by:
//   • the <label for="<host id>"> (given an id when it has none), or
//   • the host's own aria-labelledby / data-labelledby / aria-label attribute
//     (data-labelledby for a plain <div> host, which may not carry one),
// and inherits the host's aria-describedby. Clicking the label focuses the
// editor, as it would a native control.
//
// Keyboard exit (WCAG 2.1.2): Tab indents inside an editor, so pressing Esc
// arms the next Tab (or Shift+Tab) to move focus out of the editor instead.
// Any other key disarms it. The editor's input is described by a hidden hint
// saying so, and arming is announced.

import ace from 'ace-builds/src-noconflict/ace';

function hostOf(el) {
  if (typeof el === 'string') return document.getElementById(el);
  return el && el.nodeType === 1 ? el : null;
}

function labelEditor(editor, scope, hostId, attrs) {
  const input = editor && editor.textInput && editor.textInput.getElement
    ? editor.textInput.getElement()
    : null;
  if (!input) return;

  let label = null;
  if (hostId && window.CSS && CSS.escape) {
    const selector = 'label[for="' + CSS.escape(hostId) + '"]';
    label = (scope && scope.querySelector(selector)) || document.querySelector(selector);
  }
  if (label) {
    if (!label.id) label.id = hostId + '-label';
    input.setAttribute('aria-labelledby', label.id);
    // The editor replaced the host it pointed at: a dangling for= would
    // label nothing (aria-labelledby above names the editor instead).
    if (hostId && !document.getElementById(hostId)) label.removeAttribute('for');
    label.addEventListener('click', (e) => {
      e.preventDefault();
      editor.focus();
    });
  } else if (attrs.labelledby) {
    input.setAttribute('aria-labelledby', attrs.labelledby);
  } else if (attrs.label) {
    input.setAttribute('aria-label', attrs.label);
  }
  if (attrs.describedby) input.setAttribute('aria-describedby', attrs.describedby);
}

// Mark an Ace editor's input invalid (or valid) and point it at the error
// element(s) describing why, e.g. from an Alpine x-effect:
//   x-effect="aceSetInvalid(jobEditor, !jobValid, 'job-errors')"
window.aceSetInvalid = function aceSetInvalid(editor, invalid, describedby) {
  const input = editor && editor.textInput && editor.textInput.getElement
    ? editor.textInput.getElement()
    : null;
  if (!input) return;
  input.setAttribute('aria-invalid', invalid ? 'true' : 'false');
  if (!describedby) return;
  // Add or drop the error ids, keeping anything else (the keyboard hint).
  const ids = describedby.split(' ').filter(Boolean);
  let list = (input.getAttribute('aria-describedby') || '').split(' ').filter((id) => id && !ids.includes(id));
  if (invalid) list = ids.concat(list);
  if (list.length) input.setAttribute('aria-describedby', list.join(' '));
  else input.removeAttribute('aria-describedby');
};

const HINT_ID = 'knot-ace-keyboard-hint';
function hintId() {
  if (!document.getElementById(HINT_ID) && document.body) {
    const hint = document.createElement('div');
    hint.id = HINT_ID;
    hint.className = 'sr-only';
    hint.textContent = 'Tab indents. Press Escape, then Tab, to move out of the editor.';
    document.body.appendChild(hint);
  }
  return HINT_ID;
}

function keyboardExit(editor) {
  const input = editor && editor.textInput && editor.textInput.getElement
    ? editor.textInput.getElement()
    : null;
  if (!input || !editor.keyBinding) return;
  const described = (input.getAttribute('aria-describedby') || '').split(' ').filter(Boolean);
  if (!described.includes(HINT_ID)) described.push(hintId());
  input.setAttribute('aria-describedby', described.join(' '));

  let armed = false;
  const disarm = () => {
    armed = false;
    if (editor.container) editor.container.removeAttribute('data-ace-esc-armed');
  };
  // Low priority (just above Ace's defaults), so popups such as
  // autocompletion and search keep their own Escape.
  editor.keyBinding.addKeyboardHandler({
    handleKeyboard(data, hashId, keyString) {
      if (hashId === -1) { disarm(); return undefined; } // typed text
      if (keyString === 'esc' && hashId === 0) {
        // A second Escape is an ordinary one (closes the dialog).
        if (armed) { disarm(); return { command: 'null', passEvent: true }; }
        if (window.knotAnnounce) window.knotAnnounce('Press Tab to move out of the editor, or Escape again to close.');
        armed = true;
        editor.container.setAttribute('data-ace-esc-armed', '');
        return { command: 'null' };
      }
      if (armed && keyString === 'tab' && (hashId === 0 || hashId === 4)) {
        disarm();
        return { command: 'null', passEvent: true };
      }
      if (keyString && !['shift', 'ctrl', 'alt', 'cmd', 'meta'].includes(keyString)) disarm();
      return undefined;
    },
  }, 1);
  editor.on('blur', () => disarm());
}

if (ace && typeof ace.edit === 'function') {
  const origEdit = ace.edit;
  ace.edit = function (el, ...rest) {
    const host = hostOf(el);
    const hostId = host ? host.id : '';
    // Look for the label near the host first: two forms open at once (e.g.
    // a template form and a space form) can each carry a label for the same id.
    const scope = host && host.closest ? host.closest('form, [role="dialog"]') : null;
    const attrs = host
      ? {
          labelledby: host.getAttribute('aria-labelledby') || host.getAttribute('data-labelledby'),
          label: host.getAttribute('aria-label'),
          describedby: host.getAttribute('aria-describedby'),
        }
      : {};
    const editor = origEdit.call(this, el, ...rest);
    try {
      labelEditor(editor, scope, hostId, attrs);
    } catch (e) {
      /* labelling is best-effort */
    }
    try {
      keyboardExit(editor);
    } catch (e) {
      /* best-effort */
    }
    return editor;
  };
}
