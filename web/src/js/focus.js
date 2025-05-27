export const focus = {
  Element(selector) {
    setTimeout(() => {
      document.querySelector(selector).focus();
    }, 10);
  }
};
