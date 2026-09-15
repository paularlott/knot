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

  const rfb = new RFB(screen, url, {
    credentials: {},
    scaleViewport: true,
    resizeSession: false,
  });

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

  rfb.addEventListener('connect', () => settle(true));
  rfb.addEventListener('disconnect', (e) => {
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
