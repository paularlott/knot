// Sidebar star/unstar and reordering for starred nav items.
//
// The menu is server-rendered (see web/nav.go): starred items sit in their
// own block at the top, in the user's order; the rest stay in their sections
// (Workspace, Build, Admin, Extensions), and a section left empty is hidden.
//
// Clicking a star toggles membership, PUTs the new full ordering to the
// server, and reloads so the server re-renders the sections. Starred items
// reorder by dragging the handle or with the move up/down buttons (the
// single-pointer and keyboard way); the DOM is updated in place and the new
// order PUT with no reload.

// knotNavOpen reads (open undefined) or remembers whether a nav section is
// open; fallback is the section's default when nothing is remembered. Storage
// can be unavailable (private windows, blocked site data): sections then
// simply start at their defaults.
window.knotNavOpen = function knotNavOpen(key, open, fallback = false) {
  const name = 'knot:nav-open:' + key;
  try {
    if (open === undefined) {
      const v = localStorage.getItem(name);
      return v === null ? !!fallback : v === '1';
    }
    localStorage.setItem(name, open ? '1' : '0');
  } catch (_) {
    return open === undefined ? !!fallback : !!open;
  }
  return !!open;
};

(function () {
  const ENDPOINT = '/api/users/preferences/nav';

  function list() {
    return document.getElementById('nav-main-list');
  }

  // Starred URLs in current DOM order. Empty with nothing starred, which
  // correctly seeds the first star as a single-item list.
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
    return Array.from(container.querySelectorAll('.nav-starred-item[data-nav-url]'))
      .find((li) => li.getAttribute('data-nav-url') === url);
  }

  // Move buttons: the non-drag way to reorder starred items. Focus stays on
  // the button pressed so repeated presses keep moving the same item.
  function onMoveClick(container, e) {
    const btn = e.target.closest('.nav-move-btn');
    if (!btn || !container.contains(btn)) return false;
    e.preventDefault();
    const li = btn.closest('.nav-starred-item');
    const list = li && li.parentElement;
    if (!list) return true;
    if (btn.dataset.navMove === 'up' && li.previousElementSibling) {
      list.insertBefore(li, li.previousElementSibling);
    } else if (btn.dataset.navMove === 'down' && li.nextElementSibling) {
      list.insertBefore(li.nextElementSibling, li);
    } else {
      return true;
    }
    btn.focus();
    saveStarred(pinnedOrderFromDOM(container)).catch((err) => {
      console.error('failed to save nav order', err);
    });
    return true;
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
      // Only reorder relative to other starred rows — they can't be dragged
      // into a section, and section items aren't drop targets here.
      if (!target || target.getAttribute('data-nav-url') === dragUrl) return;

      const dragging = findItem(container, dragUrl);
      if (!dragging) return;

      const list = target.parentElement;
      const rect = target.getBoundingClientRect();
      const after = e.clientY - rect.top > rect.height / 2;
      if (after && target.nextSibling) {
        list.insertBefore(dragging, target.nextSibling);
      } else if (after) {
        list.appendChild(dragging);
      } else {
        list.insertBefore(dragging, target);
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

    container.addEventListener('click', (e) => {
      if (!onMoveClick(container, e)) onStarClick(container, e);
    });
    wireDrag(container);
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
