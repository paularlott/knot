// Make all modal panels draggable + resizable without changing
// how they render on open. The panel stays flex-centered by the
// backdrop (original behavior) until the user drags or resizes.
//
// Resize uses dedicated handle elements at each edge/corner — same
// approach as the AI chat window. Each handle has its own cursor so
// it always shows the right resize indicator regardless of what
// form content is underneath.
//
// Delete-confirmation dialogs (ui-modal-icon-danger) get drag but
// not resize.
//
// A panel that has been dragged or resized is remembered for the
// browser session (per page + form), so reopening the same form
// restores its position/size. Panels are constrained to the viewport
// — they can't be dragged or resized off any edge. When the browser
// window itself is resized, all remembered geometry is forgotten and
// any detached panel snaps back to its centered default.

const GEO_KEY = 'knot:modalGeo';
const MOVABLE_SELECTOR =
  '.ui-modal-panel, .ui-modal-panel-wide, .ui-modal-panel-xl, .ui-modal-panel-2xl';

function loadGeoMap() {
  try { return JSON.parse(sessionStorage.getItem(GEO_KEY) || '{}'); }
  catch { return {}; }
}

function saveGeoMap(map) {
  try { sessionStorage.setItem(GEO_KEY, JSON.stringify(map)); }
  catch {}
}

// Stable per-form key: page path + the title element's id (or its text
// as a fallback). The title id identifies the form (e.g. userModalTitle)
// and stays stable across open/close and the edit/create modes of the
// same form.
function panelKey(panel) {
  const title = panel.querySelector('.ui-modal-title');
  const name = (title && title.id) || (title && title.textContent.trim());
  return name ? (location.pathname + ':' + name) : null;
}

function enhancePanel(panel) {
  if (panel._movable) return;
  panel._movable = true;

  // Make the panel the containing block for the absolute-positioned
  // resize handles. position: relative doesn't affect flex centering.
  panel.style.position = 'relative';

  const header = panel.querySelector('.ui-modal-header');
  const isDanger = header && header.querySelector('.ui-modal-icon-danger');
  const canResize = !isDanger;

  let pos = null;  // null = still flex-centered, not yet detached
  let size = null;
  const key = panelKey(panel);

  function applyFixed(x, y, w, h) {
    panel.style.position = 'fixed';
    panel.style.left = x + 'px';
    panel.style.top = y + 'px';
    panel.style.width = w + 'px';
    if (h) panel.style.height = h + 'px';
    panel.style.maxWidth = 'none';
  }

  function detach() {
    if (pos) return;
    const r = panel.getBoundingClientRect();
    pos = { x: r.left, y: r.top };
    size = { w: panel.offsetWidth, h: panel.offsetHeight };
    applyFixed(pos.x, pos.y, size.w);
  }

  // Forget this panel's geometry and snap it back to the flex-centered
  // default. Exposed on the element so the window-resize handler can
  // reset every detached panel in one pass.
  function reset() {
    pos = null;
    size = null;
    panel.style.position = 'relative';
    panel.style.left = '';
    panel.style.top = '';
    panel.style.width = '';
    panel.style.height = '';
    panel.style.maxWidth = '';
  }
  panel._resetMovable = reset;

  function persist() {
    if (!key || !pos) return;
    const rec = { x: pos.x, y: pos.y, w: size.w };
    // Only remember height when it was explicitly set (i.e. the panel
    // was resized, not just dragged) so drag-only panels keep auto
    // height and aren't clipped on reopen.
    if (panel.style.height) rec.h = size.h;
    const map = loadGeoMap();
    map[key] = rec;
    saveGeoMap(map);
  }

  // Restore remembered geometry for this form (session memory). Clamp
  // to the current viewport in case it has shrunk since the save.
  if (key) {
    const saved = loadGeoMap()[key];
    if (saved && saved.w > 0) {
      const w = saved.w;
      const maxX = Math.max(0, window.innerWidth - w);
      const maxY = Math.max(0, window.innerHeight - (saved.h || 200));
      pos = {
        x: Math.max(0, Math.min(saved.x, maxX)),
        y: Math.max(0, Math.min(saved.y, maxY)),
      };
      size = { w, h: saved.h || 0 };
      applyFixed(pos.x, pos.y, w, saved.h || null);
    }
  }

  function isInteractive(e) {
    return e.target.closest('button, input, select, textarea, a, [contenteditable]');
  }

  // --- Drag via header ---
  if (header) {
    header.style.cursor = 'move';
    header.style.userSelect = 'none';
    header.addEventListener('mousedown', (e) => {
      if (isInteractive(e)) return;
      startDrag(e);
    });
  }

  // --- Resize handles (same pattern as AI chat window) ---
  if (canResize) {
    const handles = [
      ['e',  { top: '0', right: '0', width: '4px', height: '100%', cursor: 'e-resize' }],
      ['w',  { top: '0', left: '0', width: '4px', height: '100%', cursor: 'w-resize' }],
      ['s',  { bottom: '0', left: '0', width: '100%', height: '4px', cursor: 's-resize' }],
      ['n',  { top: '0', left: '0', width: '100%', height: '4px', cursor: 'n-resize' }],
      ['se', { bottom: '0', right: '0', width: '14px', height: '14px', cursor: 'se-resize' }],
      ['sw', { bottom: '0', left: '0', width: '14px', height: '14px', cursor: 'sw-resize' }],
      ['ne', { top: '0', right: '0', width: '14px', height: '14px', cursor: 'ne-resize' }],
      ['nw', { top: '0', left: '0', width: '14px', height: '14px', cursor: 'nw-resize' }],
    ];

    for (const [dir, style] of handles) {
      const handle = document.createElement('div');
      Object.assign(handle.style, { position: 'absolute', zIndex: '10' }, style);
      panel.appendChild(handle);

      handle.addEventListener('mousedown', (e) => {
        e.preventDefault();
        e.stopPropagation();
        startResize(e, dir);
      });
    }
  }

  function startDrag(e) {
    detach();
    const startX = e.clientX - pos.x;
    const startY = e.clientY - pos.y;
    document.body.style.userSelect = 'none';

    const onMove = (ev) => {
      // Keep the whole panel inside the viewport — it can't be dragged
      // off the right/bottom edge (nor off the top/left).
      const maxX = Math.max(0, window.innerWidth - size.w);
      const maxY = Math.max(0, window.innerHeight - size.h);
      pos.x = Math.max(0, Math.min(maxX, ev.clientX - startX));
      pos.y = Math.max(0, Math.min(maxY, ev.clientY - startY));
      panel.style.left = pos.x + 'px';
      panel.style.top = pos.y + 'px';
    };

    const onUp = () => {
      document.body.style.userSelect = '';
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      persist();
    };

    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }

  function startResize(e, dir) {
    detach();
    const startX = e.clientX;
    const startY = e.clientY;
    const startW = size.w;
    const startH = size.h;
    const startPosX = pos.x;
    const startPosY = pos.y;
    document.body.style.userSelect = 'none';
    const minW = 320;
    const minH = 200;

    const onMove = (ev) => {
      const dx = ev.clientX - startX;
      const dy = ev.clientY - startY;

      if (dir.includes('e')) {
        // Right edge can't cross the viewport's right edge.
        size.w = Math.max(minW, Math.min(window.innerWidth - pos.x, startW + dx));
      }
      if (dir.includes('s')) {
        size.h = Math.max(minH, Math.min(window.innerHeight - pos.y, startH + dy));
      }
      if (dir.includes('w')) {
        // Left edge can't cross the left viewport edge (pos.x >= 0).
        const newW = Math.max(minW, Math.min(startPosX + startW, startW - dx));
        pos.x = startPosX + (startW - newW);
        size.w = newW;
      }
      if (dir.includes('n')) {
        const newH = Math.max(minH, Math.min(startPosY + startH, startH - dy));
        pos.y = startPosY + (startH - newH);
        size.h = newH;
      }

      panel.style.width = size.w + 'px';
      panel.style.height = size.h + 'px';
      panel.style.left = pos.x + 'px';
      panel.style.top = pos.y + 'px';
    };

    const onUp = () => {
      document.body.style.userSelect = '';
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
      persist();
    };

    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }
}

function checkPanels() {
  document.querySelectorAll(MOVABLE_SELECTOR).forEach(panel => {
    if (!panel._movable && window.getComputedStyle(panel).display !== 'none') {
      enhancePanel(panel);
    }
  });
}

const observer = new MutationObserver(checkPanels);
observer.observe(document.body, {
  childList: true,
  attributes: true,
  attributeFilter: ['style', 'class'],
  subtree: true,
});

document.addEventListener('alpine:initialized', () => {
  requestAnimationFrame(checkPanels);
});

// When the browser window is resized, forget all remembered panel
// geometry and snap any moved/resized panel back to its centered
// default — simpler than re-flowing arbitrary saved positions, and
// guarantees nothing ends up off-screen.
window.addEventListener('resize', () => {
  try { sessionStorage.removeItem(GEO_KEY); } catch {}
  document.querySelectorAll(MOVABLE_SELECTOR).forEach(panel => {
    if (panel._movable && typeof panel._resetMovable === 'function') {
      panel._resetMovable();
    }
  });
});
