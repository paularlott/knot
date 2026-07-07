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

  function detach() {
    if (pos) return;
    const r = panel.getBoundingClientRect();
    pos = { x: r.left, y: r.top };
    size = { w: panel.offsetWidth, h: panel.offsetHeight };
    panel.style.position = 'fixed';
    panel.style.left = pos.x + 'px';
    panel.style.top = pos.y + 'px';
    panel.style.width = size.w + 'px';
    panel.style.maxWidth = 'none';
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
      pos.x = Math.max(0, Math.min(window.innerWidth - 80, ev.clientX - startX));
      pos.y = Math.max(0, Math.min(window.innerHeight - 40, ev.clientY - startY));
      panel.style.left = pos.x + 'px';
      panel.style.top = pos.y + 'px';
    };

    const onUp = () => {
      document.body.style.userSelect = '';
      document.removeEventListener('mousemove', onMove);
      document.removeEventListener('mouseup', onUp);
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

      if (dir.includes('e')) size.w = Math.max(minW, startW + dx);
      if (dir.includes('s')) size.h = Math.max(minH, startH + dy);
      if (dir.includes('w')) {
        const newW = Math.max(minW, startW - dx);
        pos.x = startPosX + (startW - newW);
        size.w = newW;
      }
      if (dir.includes('n')) {
        const newH = Math.max(minH, startH - dy);
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
    };

    document.addEventListener('mousemove', onMove);
    document.addEventListener('mouseup', onUp);
  }
}

function checkPanels() {
  document.querySelectorAll(
    '.ui-modal-panel, .ui-modal-panel-wide, .ui-modal-panel-xl, .ui-modal-panel-2xl'
  ).forEach(panel => {
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
