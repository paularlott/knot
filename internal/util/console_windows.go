//go:build windows

package util

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32                  = windows.NewLazySystemDLL("kernel32.dll")
	user32                    = windows.NewLazySystemDLL("user32.dll")
	procGetConsoleProcessList = kernel32.NewProc("GetConsoleProcessList")
	procFreeConsole           = kernel32.NewProc("FreeConsole")
	procGetConsoleWindow      = kernel32.NewProc("GetConsoleWindow")
	procShowWindow            = user32.NewProc("ShowWindow")
)

const swHide = 0x0

// relaunchChildEnv marks the hidden relaunch child.
const relaunchChildEnv = "_KNOT_HIDDEN_RELAUNCH"

// RelaunchHidden re-executes the process with CREATE_NO_WINDOW so the
// child never has a console at all, then the caller exits. The console
// Windows opened for the parent closes when the parent exits — the one
// event every terminal host (including Windows Terminal) honours.
//
// Desktop mode calls this unconditionally: it is only ever invoked for a
// bare `knot` (GUI usage); every subcommand keeps normal console behavior.
// Returns true when this process is the parent and should exit now.
func RelaunchHidden() bool {
	if os.Getenv(relaunchChildEnv) == "1" {
		// We are the child: no console exists, so writes to the standard
		// handles fail — send them to NUL.
		if nul, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
			os.Stdout = nul
			os.Stderr = nul
		}
		return false
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "\n  hidden restart failed (no executable path):", err)
		return false
	}

	fmt.Fprintln(os.Stderr, "\n  knot desktop is starting in the background — look for the tray icon.")

	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), relaunchChildEnv+"=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "  hidden restart failed, continuing with console:", err)
		return false
	}
	return true
}

// HideConsoleIfOwned is kept as a fallback: it closes the console window
// when no other process is attached to it.
func HideConsoleIfOwned() {
	list := make([]uint32, 64)
	ret, _, _ := procGetConsoleProcessList.Call(uintptr(len(list)), uintptr(unsafe.Pointer(&list[0])))
	if ret == 0 || ret > 1 {
		return // no console, or shared with a terminal
	}

	// Capture the window before detaching — GetConsoleWindow returns
	// nothing once we are free.
	hwnd, _, _ := procGetConsoleWindow.Call()

	procFreeConsole.Call()
	if hwnd != 0 {
		procShowWindow.Call(hwnd, swHide)
	}

	if nul, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0); err == nil {
		os.Stdout = nul
		os.Stderr = nul
	}
}
