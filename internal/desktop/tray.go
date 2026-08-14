//go:build !server

package desktop

import (
	"runtime"
	"sync"

	"github.com/gogpu/systray"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/util"
)

// tray wraps the system tray icon and menu. This is the only file importing
// gogpu/systray — the single swap point if the backend ever needs replacing.
type tray struct {
	sys      *systray.SystemTray
	stopOnce sync.Once
}

func newTray(url string, onQuit func()) *tray {
	logger := log.WithGroup("desktop")

	t := &tray{}

	menu := systray.NewMenu()
	menu.Add("Open knot UI", func() {
		if err := util.OpenBrowser(url); err != nil {
			logger.Error("failed to open browser", "error", err, "url", url)
		}
	})
	menu.AddSeparator()
	menu.Add("Quit", func() {
		onQuit()
		t.stop()
	})

	t.sys = systray.New()

	// Menu bar / notification area icons are monochrome with a transparent
	// background. macOS template icons adapt to the menu bar theme from the
	// alpha channel alone; Windows switches between the two via theme events;
	// Linux panels are predominantly dark, so use the white variant there.
	switch runtime.GOOS {
	case "darwin":
		t.sys.SetTemplateIcon(icon)
	case "windows":
		t.sys.SetIcon(icon).SetDarkModeIcon(iconWhite)
	default:
		t.sys.SetIcon(iconWhite)
	}

	t.sys.SetTooltip("knot")
	t.sys.SetMenu(menu)
	t.sys.Show()

	return t
}

// run blocks the calling goroutine, pumping the platform message loop until
// the tray is removed.
func (t *tray) run() error {
	return t.sys.Run()
}

// stop tears the tray down. Tearing down from inside the tray's own event
// handler can deadlock the platform message loop, so removal runs in its own
// goroutine; safe to call multiple times.
func (t *tray) stop() {
	t.stopOnce.Do(func() {
		go t.sys.Remove()
	})
}
