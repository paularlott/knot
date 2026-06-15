import { Terminal } from '@xterm/xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { CanvasAddon } from '@xterm/addon-canvas';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { AttachAddon } from '@xterm/addon-attach';

window.initializeTerminal = function(options) {
  const terminalElement = document.getElementById("terminal");
  const terminal = new Terminal({
    allowProposedApi: true,
    useStyle: true,
    cursorBlink: true,
    fullscreenWin: true,
    maximizeWin: true,
    screenReaderMode: true,
    cols: 128,
    fontSize: options.logView ? 13 : 15,
    fontFamily: 'JetBrains Mono, courier-new, courier, monospace',
    disableStdin: options.logView
  });

  if (options.renderer === "webgl") {
    terminal.loadAddon(new WebglAddon());
  } else {
    terminal.loadAddon(new CanvasAddon());
  }

  const protocol = (location.protocol === "https:") ? "wss://" : "ws://";
  const url = protocol + location.host + (options.logView ? `/logs/${options.spaceId}/stream` : `/proxy/spaces/${options.spaceId}/terminal/${options.shell}`);
  const ws = new WebSocket(url);

  const attachAddon = new AttachAddon(ws);
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.loadAddon(new WebLinksAddon());
  terminal.loadAddon(new Unicode11Addon());

  terminal.unicode.activeVersion = "11";

  terminal.open(terminalElement);

  let fitScheduled = false;

  function fitTerminal() {
    fitScheduled = false;
    fitAddon.fit();
  }

  function scheduleFit(delay = 0) {
    if (fitScheduled) {
      return;
    }

    fitScheduled = true;
    window.setTimeout(() => {
      window.requestAnimationFrame(fitTerminal);
    }, delay);
  }

  scheduleFit();

  ws.onclose = () => {
    if (options.logView) {
      terminal.write('\r\n\nconnection terminated, refresh to restart\n');
    } else {
      // Shell exited (server closed the WS): close the terminal popup.
      terminal.write('\r\n\n[session ended]\r\n');
      setTimeout(() => {
        try { window.close(); } catch (e) { /* ignore */ }
      }, 300);
    }
  };

  ws.onopen = () => {
    terminal.loadAddon(attachAddon);
    terminal._initialized = true;
    terminal.focus();

    terminal.element.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && e.shiftKey) {
        e.stopImmediatePropagation();
        e.preventDefault();
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(new TextEncoder().encode('\x1b\r'));
        }
      }
    }, true);

    // Do an initial resize or the terminal won't wrap correctly
    setTimeout(() => {
      fitTerminal();

      const send = new TextEncoder().encode(`\x01${JSON.stringify({cols: terminal.cols, rows: terminal.rows})}`);
      ws.send(send);
    }, 1);

    terminal.onResize((event) => {
      const size = JSON.stringify({cols: event.cols, rows: event.rows});
      const send = new TextEncoder().encode(`\x01${size}`);

      ws.send(send);
    });

    terminal.onTitleChange((title) => {
      document.title = title;
    });

    window.addEventListener('resize', () => {
      scheduleFit();
    });
  };

  function updateViewportHeight() {
    if (!window.visualViewport) {
      return;
    }

    const vh = window.visualViewport.height;
    document.documentElement.style.setProperty('--vh', `${vh}px`);
    document.documentElement.style.setProperty('--viewport-offset-top', `${window.visualViewport.offsetTop}px`);

    scheduleFit(100);
  }

  if (window.visualViewport) {
    updateViewportHeight();
    window.visualViewport.addEventListener('resize', updateViewportHeight);
    window.visualViewport.addEventListener('scroll', updateViewportHeight);
  }

  if (document.fonts?.ready) {
    document.fonts.ready.then(() => {
      scheduleFit();
    });
  }

  if (window.ResizeObserver) {
    const resizeObserver = new ResizeObserver(() => {
      scheduleFit();
    });
    resizeObserver.observe(terminalElement);
  }

  return terminal;
}
