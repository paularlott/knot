package harness

import (
	"bytes"
	"net"
	"os/exec"
	"path/filepath"
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

// freePort asks the kernel for an unused TCP port. There is an inherent race
// between closing the listener and the server binding, but tests boot servers
// sequentially and retries make collisions vanishingly rare.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func mustAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
