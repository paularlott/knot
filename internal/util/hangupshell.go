package util

import (
	"io"
	"os"
	"syscall"
	"time"
)

// ShellStopTimeout is how long a shell gets to exit after SIGHUP before it is killed.
const ShellStopTimeout = 2 * time.Second

// HangUpShell hangs up a shell whose terminal has gone away so it can save its history on
// the way out (bash, zsh and fish all do on SIGHUP), and only kills it if it hasn't exited
// within ShellStopTimeout. Killing it straight away would lose the session's history.
//
// exited must be closed once the process has been waited on. The shell's pty must stay open
// until then: closing it first makes the kernel hang the session up underneath the shell and
// bash then exits without saving its history. Its output keeps being read (and discarded)
// meanwhile, as a shell that can't flush its output to a pty nobody is reading never gets as
// far as saving. Closing tty afterwards ends that reading.
func HangUpShell(proc *os.Process, tty io.Reader, exited <-chan struct{}) {
	select {
	case <-exited:
		return
	default:
	}

	go io.Copy(io.Discard, tty)

	// Where SIGHUP can't be sent (Windows) or the process is already gone, don't wait
	if err := proc.Signal(syscall.SIGHUP); err != nil {
		proc.Kill()
		<-exited
		return
	}

	select {
	case <-exited:
	case <-time.After(ShellStopTimeout):
		proc.Kill()
		<-exited
	}
}
