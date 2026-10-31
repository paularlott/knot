export const focus = {
  // Focus the first visible match once the modal has settled. Forms stay
  // hidden behind their loading state until their data arrives, so keep
  // trying for a few seconds, but never pull focus from a field the user
  // has already moved to.
  Element(selector) {
    let tries = 0;
    const attempt = () => {
      const el = Array.from(document.querySelectorAll(selector)).find((node) => node.getClientRects().length > 0);
      if (el) {
        const active = document.activeElement;
        if (tries === 0 || !active || !active.matches('input, select, textarea, [contenteditable="true"]')) el.focus();
        return;
      }
      if (++tries < 20) setTimeout(attempt, 150);
    };
    setTimeout(attempt, 350);
  },

  // Move focus to the first field marked aria-invalid="true" inside root
  // (after Alpine has applied the failed validation's bindings), so a
  // rejected submit lands the user on what needs fixing.
  firstInvalid(root) {
    const run = () => {
      const scope = root || document;
      const el = Array.from(scope.querySelectorAll('[aria-invalid="true"]'))
        .find((node) => node.getClientRects().length > 0 && !node.disabled);
      if (el) el.focus();
    };
    if (window.Alpine && typeof window.Alpine.nextTick === 'function') window.Alpine.nextTick(run);
    else setTimeout(run, 0);
  },
};

// Make aria-hidden elements non-focusable for accessibility
document.addEventListener('DOMContentLoaded', () => {
  const observer = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
      // A modal opened over another one was hidden by the first modal's
      // x-trap.inert while it was closed; once it shows it must be reachable
      // again, or its fields stay out of the tab order and the trap can't focus it
      if (mutation.type === 'attributes' && mutation.attributeName === 'style') {
        const target = mutation.target;
        if (target.style.display === 'none') return;
        if (!target.matches('[aria-modal="true"]') && !target.querySelector(':scope > [aria-modal="true"]')) return;
        for (let el = target; el && el !== document.body; el = el.parentElement) {
          if (el.getAttribute('aria-hidden') === 'true') el.removeAttribute('aria-hidden');
        }
        return;
      }
      if (mutation.type === 'attributes' && mutation.attributeName === 'aria-hidden') {
        const target = mutation.target;
        if (target.getAttribute('aria-hidden') === 'true') {
          target.querySelectorAll('a, button, input, select, textarea, [tabindex]').forEach(el => {
            if (!el.hasAttribute('data-original-tabindex')) {
              el.setAttribute('data-original-tabindex', el.getAttribute('tabindex') || '0');
            }
            el.setAttribute('tabindex', '-1');
          });
        } else {
          target.querySelectorAll('[data-original-tabindex]').forEach(el => {
            const original = el.getAttribute('data-original-tabindex');
            if (original === '0') {
              el.removeAttribute('tabindex');
            } else {
              el.setAttribute('tabindex', original);
            }
            el.removeAttribute('data-original-tabindex');
          });
        }
      }
    });
  });
  observer.observe(document.body, { attributes: true, subtree: true, attributeFilter: ['aria-hidden', 'style'] });
});

