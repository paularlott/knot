package util

import (
	"os"
	"os/exec"
	"runtime"
)

// CanOpenBrowser reports whether OpenBrowser is likely to show a browser to
// the person at this terminal. Over SSH, or on a Unix with no graphical
// session, xdg-open may fail or start a text browser that takes over the
// terminal, so callers should show the URL instead.
func CanOpenBrowser() bool {
	return canOpenBrowser(runtime.GOOS, os.Getenv)
}

func canOpenBrowser(goos string, getenv func(string) string) bool {
	if getenv("SSH_CONNECTION") != "" || getenv("SSH_TTY") != "" {
		return false
	}
	switch goos {
	case "windows", "darwin":
		return true
	default:
		return getenv("DISPLAY") != "" || getenv("WAYLAND_DISPLAY") != ""
	}
}

// OpenBrowser opens the specified URL in the default browser of the user.
// On Windows rundll32 is used rather than `cmd /c start` because cmd is a
// console application and would flash (or leave) a console window every
// time the tray menu opens the UI.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default: // "linux", "freebsd", "openbsd", "netbsd"
		return exec.Command("xdg-open", url).Start()
	}
}
