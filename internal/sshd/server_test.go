package sshd

import (
	"bytes"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

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

// startTestServer runs the ssh handler on a local port and returns its address.
func startTestServer(t *testing.T) string {
	t.Helper()

	keyPEM, err := GenerateEd25519PrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.ParsePrivateKey([]byte(keyPEM))
	if err != nil {
		t.Fatal(err)
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &ssh.Server{Handler: defaultHandler, HostSigners: []ssh.Signer{signer}}
	go server.Serve(l)
	t.Cleanup(func() { server.Close() })

	return l.Addr().String()
}

// TestShellHistorySavedWhenSessionEnds checks a shell saves its history both when the user
// exits it and when the client drops the connection without exiting.
func TestShellHistorySavedWhenSessionEnds(t *testing.T) {
	addr := startTestServer(t)

	for _, shell := range []string{"bash", "zsh", "fish", "sh"} {
		for _, how := range []string{"exit", "drop"} {
			t.Run(shell+"/"+how, func(t *testing.T) {
				path, err := exec.LookPath(shell)
				if err != nil {
					t.Skipf("%s not installed", shell)
				}

				// A fresh home with no dotfiles, other than the empty ~/.zshrc the knot images
				// leave so zsh doesn't ask new users to set one up
				home := t.TempDir()
				if shell == "zsh" {
					os.WriteFile(filepath.Join(home, ".zshrc"), nil, 0o644)
				}
				t.Setenv("HOME", home)
				t.Setenv("PATH", os.Getenv("PATH"))
				old := preferredShell
				SetShell(shell)
				t.Cleanup(func() { SetShell(old) })

				conn, err := net.Dial("tcp", addr)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()

				sshConn, chans, reqs, err := gossh.NewClientConn(conn, addr, &gossh.ClientConfig{
					User:            "knot",
					HostKeyCallback: gossh.InsecureIgnoreHostKey(),
				})
				if err != nil {
					t.Fatal(err)
				}
				client := gossh.NewClient(sshConn, chans, reqs)

				session, err := client.NewSession()
				if err != nil {
					t.Fatal(err)
				}
				if err := session.RequestPty("xterm-256color", 24, 80, gossh.TerminalModes{}); err != nil {
					t.Fatal(err)
				}
				stdin, _ := session.StdinPipe()
				stdout, _ := session.StdoutPipe()

				var mu sync.Mutex
				var out bytes.Buffer
				go func() {
					buf := make([]byte, 4096)
					for {
						n, err := stdout.Read(buf)
						if n > 0 {
							mu.Lock()
							out.Write(buf[:n])
							mu.Unlock()
							// fish stalls at startup until the terminal answers its query
							if bytes.Contains(buf[:n], []byte("\x1b[0c")) {
								stdin.Write([]byte("\x1b[?62c"))
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

				if err := session.Shell(); err != nil {
					t.Fatal(err)
				}

				marker := "knot-ssh-marker-" + shell + "-" + how
				time.Sleep(500 * time.Millisecond)
				stdin.Write([]byte("echo " + marker + "-out\r"))
				deadline := time.Now().Add(10 * time.Second)
				for strings.Count(output(), marker+"-out") < 2 && time.Now().Before(deadline) {
					time.Sleep(50 * time.Millisecond)
				}
				if strings.Count(output(), marker+"-out") < 2 {
					t.Fatalf("shell never ran the command, output: %q", output())
				}

				if how == "exit" {
					stdin.Write([]byte("exit\r"))
					session.Wait()
				} else {
					// The shell is idle and the network goes away
					conn.Close()
				}

				if shell == "sh" {
					if link, err := filepath.EvalSymlinks(path); err == nil && filepath.Base(link) == "dash" {
						t.Skip("sh is dash, which keeps no history")
					}
				}

				// The shell is hung up shortly after the connection drops
				deadline = time.Now().Add(10 * time.Second)
				for !historyContains(home, marker) && time.Now().Before(deadline) {
					time.Sleep(100 * time.Millisecond)
				}
				if !historyContains(home, marker) {
					t.Errorf("%s did not save its history when the session ended by %s", shell, how)
				}
			})
		}
	}
}
