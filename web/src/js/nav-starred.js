// Sidebar pin/unpin + drag-to-reorder for starred (pinned) nav items.
//
// The menu is server-rendered in two modes (see web/nav.go):
//   • Mode A (no pins) — default layout; every item shows an outline star.
//   • Mode B (≥1 pin)  — pinned items sit on top with a drag handle; everything
//     else collapses into "More".
//
// Clicking a star toggles membership, POSTs the new full ordering to the
// server, and reloads so the server re-renders the right mode. Dragging a
// pinned item reorders it live in the DOM; on drop the new order is POSTed
// with no reload (the DOM already matches the server state).

(function () {
  const ENDPOINT = '/api/users/preferences/nav';

  function list() {
    return document.getElementById('nav-main-list');
  }

  // Pinned URLs in current DOM order (the starred rows at the top). Empty in
  // Mode A, which correctly seeds the first pin as a single-item list.
  function pinnedOrderFromDOM(container) {
    return Array.from(container.querySelectorAll('.nav-starred-item[data-nav-url]'))
      .map((li) => li.getAttribute('data-nav-url'));
  }

  function saveStarred(order) {
    return fetch(ENDPOINT, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ starred: order }),
    });
  }

  function onStarClick(container, e) {
    const btn = e.target.closest('.nav-star-btn');
    if (!btn || !container.contains(btn)) return;
    e.preventDefault();

    const url = btn.getAttribute('data-nav-url');
    const order = pinnedOrderFromDOM(container);
    const i = order.indexOf(url);
    if (i >= 0) {
      order.splice(i, 1); // unpin
    } else {
      order.push(url); // pin (appends to the bottom of the pinned set)
    }

    saveStarred(order).then((resp) => {
      if (resp.ok) {
        location.reload();
      } else {
        console.error('failed to save nav preferences', resp.status);
      }
    });
  }

  function findItem(container, url) {
    return container.querySelector('.nav-starred-item[data-nav-url="' + url + '"]');
  }

  function wireDrag(container) {
    let dragUrl = null;

    container.addEventListener('dragstart', (e) => {
      const li = e.target.closest('.nav-starred-item');
      if (!li) return;
      dragUrl = li.getAttribute('data-nav-url');
      li.classList.add('dragging');
      e.dataTransfer.effectAllowed = 'move';
      try { e.dataTransfer.setData('text/plain', dragUrl); } catch (_) {}
    });

    container.addEventListener('dragend', () => {
      const dragging = container.querySelector('.nav-starred-item.dragging');
      if (dragging) dragging.classList.remove('dragging');
      if (!dragUrl) return;
      dragUrl = null;
      // Persist the order now reflected in the DOM.
      saveStarred(pinnedOrderFromDOM(container)).catch((err) => {
        console.error('failed to save nav order', err);
      });
    });

    // Always mark the container as a valid drop target (preventDefault on every
    // dragover) and accept the drop. Without this the browser treats a drop
    // over a gap or over the dragged row itself as cancelled and animates the
    // drag ghost back to its source. The live reorder still only fires when
    // hovering another pinned row.
    container.addEventListener('dragover', (e) => {
      if (!dragUrl) return;
      e.preventDefault();
      e.dataTransfer.dropEffect = 'move';

      const target = e.target.closest('.nav-starred-item');
      // Only reorder relative to other pinned rows — pins can't be dragged
      // into "More" and More items aren't drop targets here.
      if (!target || target.getAttribute('data-nav-url') === dragUrl) return;

      const dragging = findItem(container, dragUrl);
      if (!dragging) return;

      const rect = target.getBoundingClientRect();
      const after = e.clientY - rect.top > rect.height / 2;
      if (after && target.nextSibling) {
        container.insertBefore(dragging, target.nextSibling);
      } else if (after) {
        container.appendChild(dragging);
      } else {
        container.insertBefore(dragging, target);
      }
    });

    // Accept the drop so the drag ghost vanishes in place instead of snapping
    // back to the source. The DOM is already correct from the live dragover
    // moves; the final order is persisted in dragend below.
    container.addEventListener('drop', (e) => {
      if (!dragUrl) return;
      e.preventDefault();
    });
  }

  function init() {
    const container = list();
    if (!container || container.dataset.navReady) return;
    container.dataset.navReady = '1';

    container.addEventListener('click', (e) => onStarClick(container, e));
    wireDrag(container);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
