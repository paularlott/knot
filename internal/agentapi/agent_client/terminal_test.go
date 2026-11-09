package agent_client

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/paularlott/knot/internal/util"
)

// historyContains reports whether any file under home holds marker.
func historyContains(home, marker string) bool {
	found := false
	filepath.Walk(home, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if data, err := os.ReadFile(path); err == nil && bytes.Contains(data, []byte(marker)) {
			found = true
		}
		return nil
	})
	return found
}

// TestStopShellSavesHistory checks that closing a terminal hangs the shell up rather than
// killing it, so the shell saves the session's history. The shell gets a fresh home with no
// dotfiles, so this covers what a shell does out of the box on the platform running the test.
func TestStopShellSavesHistory(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "sh"} {
		t.Run(shell, func(t *testing.T) {
			path, err := exec.LookPath(shell)
			if err != nil {
				t.Skipf("%s not installed", shell)
			}

			home := t.TempDir()
			marker := "knot-history-marker-" + shell

			// zsh asks new users to set up their ~/.zshrc on first run; the knot images
			// leave an empty one so that doesn't stall the terminal, so do the same
			if shell == "zsh" {
				os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0o644)
			}

			cmd := exec.Command(path, "-l")
			cmd.Dir = home
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TERM=xterm-256color", "LANG=C.UTF-8", "USER=knot"}
			tty, err := pty.Start(cmd)
			if err != nil {
				t.Fatalf("failed to start shell: %v", err)
			}

			var mu sync.Mutex
			var out bytes.Buffer
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := tty.Read(buf)
					if n > 0 {
						mu.Lock()
						out.Write(buf[:n])
						mu.Unlock()
						// fish asks the terminal what it is at startup and stalls until it
						// answers, as a real terminal emulator does
						if bytes.Contains(buf[:n], []byte("\x1b[0c")) {
							tty.Write([]byte("\x1b[?62c"))
						}
					}
					if err != nil {
						return
					}
				}
			}()
			output := func() string {
				mu.Lock()
				defer mu.Unlock()
				return out.String()
			}

			// Run a command and wait for its output (the typed command echoes the marker once,
			// the output is the second time) so it is in the shell's history before closing
			time.Sleep(500 * time.Millisecond)
			tty.Write([]byte("echo " + marker + "-out\r"))
			deadline := time.Now().Add(10 * time.Second)
			for strings.Count(output(), marker+"-out") < 2 && time.Now().Before(deadline) {
				time.Sleep(50 * time.Millisecond)
			}
			if strings.Count(output(), marker+"-out") < 2 {
				t.Fatalf("shell never ran the command, output: %q", output())
			}

			start := time.Now()
			stopShell(cmd, tty)
			if elapsed := time.Since(start); elapsed >= util.ShellStopTimeout {
				t.Errorf("shell had to be killed, took %v to stop", elapsed)
			}

			// dash, the usual sh on Debian and Ubuntu, has no history to save
			if shell == "sh" {
				if link, err := filepath.EvalSymlinks(path); err == nil && filepath.Base(link) == "dash" {
					t.Skip("sh is dash, which keeps no history")
				}
			}

			if !historyContains(home, marker) {
				t.Errorf("%s did not save its history to %s", shell, home)
			}
		})
	}
}

// TestStopShellKillsStuckShell checks that a shell ignoring SIGHUP is still stopped.
func TestStopShellKillsStuckShell(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not installed")
	}

	cmd := exec.Command(sh, "-c", "trap '' HUP; while :; do sleep 1; done")
	tty, err := pty.Start(cmd)
	if err != nil {
		t.Fatalf("failed to start shell: %v", err)
	}
	time.Sleep(300 * time.Millisecond)

	start := time.Now()
	stopShell(cmd, tty)
	if elapsed := time.Since(start); elapsed < util.ShellStopTimeout || elapsed > util.ShellStopTimeout+3*time.Second {
		t.Errorf("expected the shell to be killed after ~%v, took %v", util.ShellStopTimeout, elapsed)
	}
}
