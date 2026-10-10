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
  if (invalid && describedby) input.setAttribute('aria-describedby', describedby);
  else if (describedby && input.getAttribute('aria-describedby') === describedby) input.removeAttribute('aria-describedby');
};

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
    return editor;
  };
}
