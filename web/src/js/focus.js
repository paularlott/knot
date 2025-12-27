export const focus = {
  Element(selector) {
    setTimeout(() => {
      const el = document.querySelector(selector);
      if (el) {
        el.focus();
      }
    }, 350);
  }
};

// Make aria-hidden elements non-focusable for accessibility
document.addEventListener('DOMContentLoaded', () => {
  const observer = new MutationObserver((mutations) => {
    mutations.forEach((mutation) => {
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
  observer.observe(document.body, { attributes: true, subtree: true, attributeFilter: ['aria-hidden'] });
});

