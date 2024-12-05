import '../terminal/terminal.less'

import { Terminal } from '@xterm/xterm';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { FitAddon } from '@xterm/addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { CanvasAddon } from '@xterm/addon-canvas';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { AttachAddon } from '@xterm/addon-attach';
import { SerializeAddon } from '@xterm/addon-serialize';

window.initializeTerminal = function(options) {
  var terminal = new Terminal({
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

  var protocol = (location.protocol === "https:") ? "wss://" : "ws://";
  var url = protocol + location.host + (options.logView ? "/logs/" + options.spaceId + "/stream" : "/proxy/spaces/" + options.spaceId + "/terminal/" + options.shell);
  var ws = new WebSocket(url);

  var attachAddon = new AttachAddon(ws);
  var fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  terminal.loadAddon(new WebLinksAddon());
  terminal.loadAddon(new Unicode11Addon());
  terminal.loadAddon(new SerializeAddon());

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

    setTimeout(function() {fitAddon.fit();});

    terminal.onResize((event) => {
      var rows = event.rows;
      var cols = event.cols;
      var size = JSON.stringify({cols: cols, rows: rows + 1});
      var send = new TextEncoder().encode("\x01" + size);

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
