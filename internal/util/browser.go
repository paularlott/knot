package util

import (
	"os/exec"
	"runtime"
)

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
