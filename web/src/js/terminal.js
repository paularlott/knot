import '../terminal/terminal.less'

import { Terminal } from '@xterm/xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { CanvasAddon } from '@xterm/addon-canvas';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { AttachAddon } from '@xterm/addon-attach';

window.initializeTerminal = function(options) {
  let terminal = new Terminal({
    allowProposedApi: true,
    screenKeys: true,
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

  if (options.renderer == "webgl") {
    terminal.loadAddon(new WebglAddon());
  } else {
    terminal.loadAddon(new CanvasAddon());
  }

  let protocol = (location.protocol === "https:") ? "wss://" : "ws://";
  let url = protocol + location.host + (options.logView ? "/logs/" + options.spaceId + "/stream" : "/proxy/spaces/" + options.spaceId + "/terminal/" + options.shell);
  let ws = new WebSocket(url);

  let attachAddon = new AttachAddon(ws);
  let fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.loadAddon(new WebLinksAddon());
  terminal.loadAddon(new Unicode11Addon());

  terminal.unicode.activeVersion = "11";

  terminal.open(document.getElementById("terminal"));

  fitAddon.fit();

  ws.onclose = function(event) {
    terminal.write('\r\n\nconnection terminated, refresh to restart\n')
  };

  ws.onopen = function() {
    terminal.loadAddon(attachAddon);
    terminal._initialized = true;
    terminal.focus();

    // Do an initial resize or the terminal won't wrap correctly
    setTimeout(function() {
      fitAddon.fit();

      send = new TextEncoder().encode("\x01" + JSON.stringify({cols: terminal.cols, rows: terminal.rows}));
      ws.send(send);
    }, 1);

    terminal.onResize((event) => {
      let size = JSON.stringify({cols: event.cols, rows: event.rows});
      let send = new TextEncoder().encode("\x01" + size);

      ws.send(send);
    });

    terminal.onTitleChange((title) => {
      document.title = title;
    });

    window.onresize = function() {
      fitAddon.fit();
    };
  };
}
