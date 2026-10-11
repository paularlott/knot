// Shared page services:
//
//   knotLive — live updates (server events and the pages' refresh timers) can
//   be paused from the header, so nothing on a page changes by itself while
//   someone is reading it (WCAG 2.2.2). While paused, server events are held
//   (the latest per kind and item) and replayed on resume; timers check
//   knotLive.paused and skip their tick. The choice lasts for the browser tab.
//
//       knotLive.paused            -> boolean
//       knotLive.pause() / resume() / toggle()
//       knotLive.onResume(fn)      -> unsubscribe; fn runs after a resume
//
//   knotAnnounce(message, { assertive }) — tells screen readers about a change
//   that happened without the user doing anything (a space starting, a toast).
//   Uses two regions that exist from page load, so announcements are not lost.
//
//   knotErrorMessage(action, err) -> Promise<string>
//   knotError(action, err)        -> shows it as an error toast
//   `action` says what was being done, e.g. "start the space"; `err` may be a
//   fetch Response, an Error (a failed fetch is a TypeError), a string, or a
//   parsed `{ error }` body. The message says what failed and, where it
//   helps, what to do next, in plain words: "Couldn't start the space. You
//   don't have permission to do that."
//
//   Shortcut hints: elements with data-shortcut="mod+k" (or "mod+shift+k")
//   are filled in with the platform's keys (⌘ on Apple, Ctrl elsewhere).

import Alpine from 'alpinejs';

const PAUSE_KEY = 'knot:live-paused';

function readPaused() {
  try { return sessionStorage.getItem(PAUSE_KEY) === '1'; } catch (_) { return false; }
}

const resumeHandlers = new Set();

// Reactive, so the header button follows it ($store.live).
export const knotLive = Alpine.reactive({
  paused: readPaused(),
  held: 0, // events held while paused, for the header badge
  pause() { this.set(true); },
  resume() { this.set(false); },
  toggle() { this.set(!this.paused); },
  set(paused) {
    if (this.paused === paused) return;
    this.paused = paused;
    try { sessionStorage.setItem(PAUSE_KEY, paused ? '1' : '0'); } catch (_) { /* storage unavailable */ }
    knotAnnounce(paused
      ? 'Live updates paused. The page will not change until you resume them.'
      : 'Live updates resumed.');
    if (!paused) {
      this.held = 0;
      resumeHandlers.forEach((fn) => { try { fn(); } catch (e) { console.error(e); } });
    }
    window.dispatchEvent(new CustomEvent('knot:live-changed', { detail: { paused } }));
  },
  onResume(fn) {
    resumeHandlers.add(fn);
    return () => resumeHandlers.delete(fn);
  },
});
window.knotLive = knotLive;
document.addEventListener('alpine:init', () => Alpine.store('live', knotLive));

// ---- announcements ------------------------------------------------------

let regions = null;
function liveRegions() {
  if (regions && regions.polite.isConnected) return regions;
  const make = (politeness) => {
    const el = document.createElement('div');
    el.className = 'sr-only';
    el.setAttribute('aria-live', politeness);
    el.setAttribute('aria-atomic', 'true');
    if (politeness === 'assertive') el.setAttribute('role', 'alert');
    else el.setAttribute('role', 'status');
    document.body.appendChild(el);
    return el;
  };
  regions = { polite: make('polite'), assertive: make('assertive') };
  return regions;
}

let clearTimer = null;
export function knotAnnounce(message, { assertive = false } = {}) {
  const text = String(message == null ? '' : message).trim();
  if (!text || !document.body) return;
  const r = liveRegions();
  const el = assertive ? r.assertive : r.polite;
  // Clear then set on the next frame so a repeated message is read again.
  el.textContent = '';
  requestAnimationFrame(() => { el.textContent = text; });
  clearTimeout(clearTimer);
  clearTimer = setTimeout(() => { r.polite.textContent = ''; r.assertive.textContent = ''; }, 7000);
}
window.knotAnnounce = knotAnnounce;

document.addEventListener('DOMContentLoaded', () => liveRegions());

// ---- friendly errors ----------------------------------------------------

function sentence(text) {
  let s = String(text || '').trim().replace(/^error[:!]?\s*/i, '');
  if (!s) return '';
  s = s.charAt(0).toUpperCase() + s.slice(1);
  if (!/[.!?]$/.test(s)) s += '.';
  return s;
}

const BY_STATUS = {
  400: "Something in the request wasn't accepted. Check the details and try again.",
  401: 'Your session has ended. Sign in again to continue.',
  403: "You don't have permission to do that.",
  404: 'It no longer exists. It may have been deleted.',
  409: 'It was changed by someone else at the same time. Refresh and try again.',
  413: 'That is too large for the server to accept.',
  429: 'Too many requests at once. Wait a moment, then try again.',
  503: 'The server is busy or restarting. Try again in a moment.',
};

async function detailOf(err) {
  if (err == null) return { text: '' };
  if (typeof Response !== 'undefined' && err instanceof Response) {
    let server = '';
    try {
      const body = await err.clone().text();
      try {
        const data = JSON.parse(body);
        server = (data && (data.error || data.message)) || '';
      } catch (_) {
        // Plain text bodies are used as-is, unless they look like an HTML page.
        if (body && body.length < 300 && !/^\s*</.test(body)) server = body;
      }
    } catch (_) { /* unreadable body */ }
    return { status: err.status, text: server };
  }
  if (err instanceof TypeError && /fetch|network|load failed/i.test(err.message)) {
    return { network: true };
  }
  if (err instanceof Error) return { status: err.status, text: err.message };
  if (typeof err === 'object') return { status: err.status, text: err.error || err.message || '' };
  return { text: String(err) };
}

export async function knotErrorMessage(action, err) {
  const d = await detailOf(err);
  let reason;
  if (d.network) {
    reason = "The server couldn't be reached. Check your connection and try again.";
  } else if (d.text && !/^(undefined|null|\[object object\])$/i.test(d.text)) {
    reason = sentence(d.text);
    if (d.status === 401) reason = BY_STATUS[401];
  } else if (d.status && BY_STATUS[d.status]) {
    reason = BY_STATUS[d.status];
  } else if (d.status >= 500) {
    reason = 'The server ran into a problem. Try again, and check the server log if it keeps happening.';
  } else {
    reason = 'Something went wrong. Try again.';
  }
  return action ? `Couldn't ${action}. ${reason}` : reason;
}
window.knotErrorMessage = knotErrorMessage;

export async function knotError(action, err) {
  const msg = await knotErrorMessage(action, err);
  if (window.knotToast) window.knotToast(msg, 'error');
  else window.dispatchEvent(new CustomEvent('show-alert', { detail: { type: 'error', msg } }));
  return msg;
}
window.knotError = knotError;

// ---- shortcut hints -----------------------------------------------------

// userAgentData says "macOS", navigator.platform "MacIntel": match either.
export const isApple = /mac|iphone|ipad|ipod/i.test(
  (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || navigator.userAgent,
);

export function shortcutLabel(spec) {
  return spec.split('+').map((k) => {
    if (k === 'mod') return isApple ? '⌘' : 'Ctrl';
    if (k === 'shift') return isApple ? '⇧' : 'Shift';
    if (k === 'alt') return isApple ? '⌥' : 'Alt';
    return k.toUpperCase();
  }).join(isApple ? ' ' : '+');
}
window.knotShortcutLabel = shortcutLabel;

function fillShortcuts(root = document) {
  root.querySelectorAll('[data-shortcut]').forEach((el) => {
    el.textContent = shortcutLabel(el.dataset.shortcut);
  });
  // Name both ways of pressing it (⌘ or ⌥ on a Mac, Ctrl or Alt elsewhere)
  // in the tooltip of the control a hint belongs to.
  root.querySelectorAll('[data-shortcut-title]').forEach((el) => {
    const spec = el.dataset.shortcutTitle;
    el.title = el.dataset.shortcutLabel + ' (' + shortcutLabel(spec) + ' or ' + shortcutLabel(spec.replace('mod', 'alt')) + ')';
  });
}
document.addEventListener('DOMContentLoaded', () => fillShortcuts());
