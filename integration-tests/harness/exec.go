package harness

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"net"
	"os/exec"
	"path/filepath"
	"sync"
)

func runGo(dir string, args ...string) (string, error) {
	return runGoEnv(dir, nil, args...)
}

func runGoEnv(dir string, env map[string]string, args ...string) (string, error) {
	cmd := exec.Command("go", args...)
	cmd.Dir = dir
	for k, v := range env {
		cmd.Env = append(cmd.Environ(), k+"="+v)
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Ports FreePort hands out: below the kernel's ephemeral range (49152+ on
// macOS, 32768+ on Linux), so an outgoing connection — a server's gossip
// dialling its peer, say — can't take one between FreePort returning it and
// a server binding it; and below 30000, where the Pro peer mesh publishes.
const (
	firstTestPort = 21000
	lastTestPort  = 29999
)

var (
	portMu   sync.Mutex
	nextPort = firstTestPort + rand.IntN(lastTestPort-firstTestPort+1)
)

// FreePort returns a port no one is using, never the same one twice in a
// run. It's checked free for TCP on every interface and on loopback, and
// for UDP, which the cluster's gossip binds too.
func FreePort() (int, error) {
	portMu.Lock()
	defer portMu.Unlock()
	for range lastTestPort - firstTestPort + 1 {
		port := nextPort
		if nextPort++; nextPort > lastTestPort {
			nextPort = firstTestPort
		}
		if portFree(port) {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port between %d and %d", firstTestPort, lastTestPort)
}

func portFree(port int) bool {
	for _, host := range []string{"0.0.0.0", "127.0.0.1"} {
		l, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
		if err != nil {
			return false
		}
		l.Close()
	}
	pc, err := net.ListenPacket("udp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return false
	}
	pc.Close()
	return true
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
