// Warn before the web session times out (WCAG 2.2.1).
//
// A session lasts SESSION_TTL after the last authenticated request (the
// server extends it on every request: internal/middleware/authmiddleware.go,
// model.SessionExpiryDuration). The page notes when it last spoke to the
// server — fetches to /api/ — in localStorage, shared by every tab, so work
// in one tab keeps the others from warning. WARN_BEFORE the end a dialog
// counts down and offers to stay signed in (a /api/ping extends the session)
// or sign out now. If the time runs out the page goes to the sign-in page,
// which brings the user back here afterwards.

import Alpine from 'alpinejs';

const SESSION_TTL = 2 * 60 * 60 * 1000; // keep in step with model.SessionExpiryDuration
const WARN_BEFORE = 2 * 60 * 1000;
const KEY = 'knot:last-activity';

let memoryLast = Date.now();

function lastActivity() {
  try {
    const v = parseInt(localStorage.getItem(KEY) || '', 10);
    if (v > memoryLast) memoryLast = v;
  } catch (_) { /* storage unavailable: this tab's own record */ }
  return memoryLast;
}

function touch() {
  memoryLast = Date.now();
  try { localStorage.setItem(KEY, String(memoryLast)); } catch (_) { /* ignore */ }
}

const state = Alpine.reactive({
  show: false,
  remaining: 0, // seconds
  get label() {
    const m = Math.floor(this.remaining / 60);
    const s = String(this.remaining % 60).padStart(2, '0');
    return m + ':' + s;
  },
  async stay() {
    try {
      const res = await origFetch('/api/ping', { headers: { 'Content-Type': 'application/json' } });
      if (res.status === 401) return expire();
      touch();
      this.show = false;
      if (window.knotAnnounce) window.knotAnnounce('You are still signed in.');
    } catch (e) {
      if (window.knotError) window.knotError('keep you signed in', e);
    }
  },
  signOut() {
    window.location.href = '/logout';
  },
});

function expire() {
  window.location.href = '/login?redirect=' + encodeURIComponent(location.pathname + location.search);
}

// Count same-origin API calls as activity (the server extends the session on
// each one). Wrapping fetch keeps every page covered without changing them.
const origFetch = window.fetch.bind(window);
window.fetch = function (input, init) {
  const p = origFetch(input, init);
  try {
    const url = typeof input === 'string' ? input : (input && input.url) || '';
    const sameOriginApi = url.startsWith('/api/') || url.startsWith(location.origin + '/api/');
    if (sameOriginApi) {
      p.then((res) => { if (res && res.status !== 401) touch(); }).catch(() => {});
    }
  } catch (_) { /* never break fetch */ }
  return p;
};

function tick() {
  // Only pages that need a session: the sign-in pages have no dialog.
  if (!document.querySelector('[data-session-warning]')) return;
  const left = lastActivity() + SESSION_TTL - Date.now();
  if (left <= 0) return expire();
  if (left <= WARN_BEFORE) {
    const wasShown = state.show;
    state.remaining = Math.ceil(left / 1000);
    state.show = true;
    if (!wasShown && window.knotAnnounce) {
      window.knotAnnounce('Your session will end in ' + Math.ceil(left / 60000) + ' minutes because of inactivity.', { assertive: true });
    }
  } else if (state.show) {
    // Activity elsewhere (another tab) extended the session.
    state.show = false;
  }
}

touch(); // loading this page was a request
document.addEventListener('alpine:init', () => Alpine.store('sessionWarning', state));
setInterval(tick, 1000);
