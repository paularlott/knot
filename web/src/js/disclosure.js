// Accessibility fixes for @awcodes/alpine-floating-ui dropdown panels.
//
// Row-action dropdowns and the user menu are simple disclosure widgets: a
// trigger button with aria-expanded that shows/hides a panel of ordinary
// buttons and links (no menu roles, so no arrow-key contract to honour).
//
// The x-float directive marks every panel role="dialog" aria-modal="true",
// which is wrong for a non-modal dropdown (and for tooltips), so this wraps
// the directive to:
//   • drop the dialog role / aria-modal it adds;
//   • keep aria-expanded on the trigger (the plugin already toggles it on
//     open/close; make sure it is present from the start);
//   • close the panel on Escape and return focus to its trigger;
//   • remember the trigger of the panel closed most recently, so a dialog
//     opened from a panel item can return focus there (the item itself is
//     hidden by then).

import AlpineFloatingUI from '@awcodes/alpine-floating-ui';

let lastTrigger = null;
let lastClosedAt = 0;

// Trigger of a dropdown panel closed within the last second, if any.
export function recentDisclosureTrigger() {
  if (lastTrigger && Date.now() - lastClosedAt < 1000 && lastTrigger.isConnected) return lastTrigger;
  return null;
}

function enhancePanel(panel, cleanup) {
  // Only interactive panels (opened from a $refs.<name>.toggle/open trigger)
  // get the disclosure behaviour; tooltips just lose the dialog role.
  panel.removeAttribute('aria-modal');
  if (panel.getAttribute('role') === 'dialog') panel.removeAttribute('role');
  if (typeof panel.close !== 'function') return;

  panel.setAttribute('data-knot-disclosure', '');

  const refName = panel.getAttribute('x-ref');
  const component = panel.parentElement && panel.parentElement.closest('[x-data]');
  if (component && refName) {
    // Triggers: any element whose @click / x-on:click (with or without
    // modifiers) starts with "$refs.<refName>".
    const prefix = '$refs.' + refName + '.';
    component.querySelectorAll('button, a, [role="button"]').forEach((t) => {
      const isTrigger = Array.from(t.attributes).some((a) =>
        (a.name.startsWith('@click') || a.name.startsWith('x-on:click')) &&
        a.value.trim().startsWith(prefix));
      if (!isTrigger || panel.contains(t)) return;
      if (!t.hasAttribute('aria-expanded')) t.setAttribute('aria-expanded', 'false');
      t.removeAttribute('aria-haspopup');
    });
  }

  const origClose = panel.close;
  panel.close = function (...args) {
    const wasShown = panel._x_isShown;
    const result = origClose.apply(this, args);
    if (wasShown && panel.trigger) {
      lastTrigger = panel.trigger;
      lastClosedAt = Date.now();
    }
    return result;
  };

  // Registered at init, i.e. before the plugin's own Escape listener (which
  // is only added on open), so this runs first and can restore focus.
  const onKeydown = (e) => {
    if (e.key !== 'Escape' || !panel._x_isShown) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    const trigger = panel.trigger;
    panel.close();
    if (trigger && typeof trigger.focus === 'function') trigger.focus();
  };
  window.addEventListener('keydown', onKeydown, true);
  cleanup(() => window.removeEventListener('keydown', onKeydown, true));
}

// Alpine plugin: registers x-float (via @awcodes/alpine-floating-ui) with the
// fixes above applied to every panel.
export default function floatingDisclosure(Alpine) {
  const origDirective = Alpine.directive;
  Alpine.directive = function (name, callback, ...rest) {
    if (name === 'float') {
      const inner = callback;
      callback = function (el, directive, utilities) {
        const r = inner.call(this, el, directive, utilities);
        enhancePanel(el, utilities.cleanup);
        return r;
      };
    }
    return origDirective.call(this, name, callback, ...rest);
  };
  try {
    AlpineFloatingUI(Alpine);
  } finally {
    Alpine.directive = origDirective;
  }
}
