// noVNC viewer for a KVM space's QEMU display. The page (vnc.tmpl) provides
// a #screen element carrying the space id; the websocket is bridged to the
// VM's local VNC port by the server's /proxy/spaces/{id}/vnc handler.
//
// noVNC is MPL-2.0 — see the @novnc/novnc package for its source and
// license.
import RFB from '@novnc/novnc';

function initVNC() {
  const screen = document.getElementById('screen');
  if (!screen) {
    return;
  }

  const status = document.getElementById('vnc-status');

  const showStatus = (text) => {
    status.textContent = text;
    status.style.display = 'flex';
  };
  const hideStatus = () => {
    status.style.display = 'none';
  };

  const spaceId = screen.dataset.spaceId;
  const protocol = (location.protocol === 'https:') ? 'wss://' : 'ws://';
  const url = `${protocol}${location.host}/proxy/spaces/${spaceId}/vnc`;

  // Fit (scaled to the window) or 1:1 (native pixels, window sized to the
  // display); remembered across opens.
  const SCALE_MODE_KEY = '_vnc_scale_mode';
  const fit = localStorage.getItem(SCALE_MODE_KEY) !== '1:1';

  const rfb = new RFB(screen, url, {
    credentials: {},
  });
  // noVNC's constructor only honours credentials/shared/repeaterID/wsProtocols;
  // the viewport behaviour must be set as properties afterwards or it silently
  // stays at the default (native size, no scaling).
  rfb.scaleViewport = fit;

  const resLabel = document.getElementById('vnc-res');
  const scaleBtn = document.getElementById('vnc-scale');
  const updateScaleButton = () => {
    scaleBtn.textContent = rfb.scaleViewport ? '1:1' : 'Fit';
    scaleBtn.title = rfb.scaleViewport
      ? 'Show the display at its native size, resizing the window to fit it'
      : 'Scale the display to fit the window';
  };
  scaleBtn.addEventListener('click', () => {
    rfb.scaleViewport = !rfb.scaleViewport;
    localStorage.setItem(SCALE_MODE_KEY, rfb.scaleViewport ? 'fit' : '1:1');
    updateScaleButton();
    // Entering 1:1 means native pixels: size the window to the display so
    // it shows whole (capped at the screen); Fit scales to any window, so
    // it keeps the size the user has.
    if (!rfb.scaleViewport && lastW && lastH) {
      resizeWindowToDisplay(lastW, lastH);
    }
  });
  updateScaleButton();

  // The popup opens at a fixed size, but the VM may be running at anything
  // from 720x400 (GRUB) to a full desktop resolution. Follow the display's
  // native resolution — resize the window to it, capped at the screen —
  // whenever it changes, including on first connect. Windows not opened by
  // script (tabs) can't be resized and just stay scaled/scrolling.
  const resizeWindowToDisplay = (w, h) => {
    if (!window.opener) {
      return;
    }
    // Window chrome (title bar) plus the in-page toolbar both sit above the
    // canvas area; without the toolbar's height a 1:1 window is short by
    // exactly that and grows a scrollbar.
    const bar = document.getElementById('vnc-bar');
    const barH = bar ? bar.offsetHeight : 0;
    const chromeW = Math.max(0, window.outerWidth - window.innerWidth);
    const chromeH = Math.max(0, window.outerHeight - window.innerHeight);
    const targetW = Math.min(w + chromeW, window.screen.availWidth - 16);
    const targetH = Math.min(h + chromeH + barH, window.screen.availHeight - 16);
    if (targetW < 400 || targetH < 300) {
      return;
    }
    window.resizeTo(targetW, targetH);
  };

  // noVNC keeps the canvas attribute size at the framebuffer's native
  // resolution (CSS style does the scaling), so watching it is a reliable
  // resolution signal; there is no event for a plain framebuffer resize.
  let connected = false;
  let lastW = 0;
  let lastH = 0;
  setInterval(() => {
    if (!connected) {
      return;
    }
    const canvas = screen.querySelector('canvas');
    if (!canvas || !canvas.width || !canvas.height) {
      return;
    }
    if (canvas.width === lastW && canvas.height === lastH) {
      return;
    }
    lastW = canvas.width;
    lastH = canvas.height;
    resLabel.textContent = `${lastW}×${lastH}`;
    resizeWindowToDisplay(lastW, lastH);
  }, 400);

  // No event means no feedback: surface anything noVNC waits on or fails
  // with, and time out a handshake that never completes so the overlay
  // always tells the user something.
  let settled = false;
  const settle = (hide, text) => {
    settled = true;
    clearTimeout(connectTimer);
    if (hide) {
      hideStatus();
    } else if (text) {
      showStatus(text);
    }
  };

  const connectTimer = setTimeout(() => {
    if (!settled) {
      settle('Still waiting for the VM\'s display — the screen stays blank until the guest draws one (a server image shows text only after the console writes; a desktop shows its login once installed). Refresh to retry.');
    }
  }, 15000);

  rfb.addEventListener('connect', () => {
    connected = true;
    settle(true);
  });
  rfb.addEventListener('disconnect', (e) => {
    connected = false;
    settle(e.detail.clean
      ? 'Disconnected — refresh to reconnect.'
      : 'Disconnected — the VM\'s display is unreachable (it may be stopped, or the server could not reach it). Refresh to retry.');
  });
  rfb.addEventListener('credentialsrequired', () => {
    settle('The VM\'s display asked for a password, which the QEMU display is not configured to require — check the domain\'s graphics configuration.');
  });
  rfb.addEventListener('securityfailure', (e) => {
    settle('The VM\'s display rejected the connection: ' + (e.detail?.reason || 'security negotiation failed') + '.');
  });
}

if (document.readyState === 'loading') {
  document.addEventListener('DOMContentLoaded', initVNC);
} else {
  initVNC();
}
