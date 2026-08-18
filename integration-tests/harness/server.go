package harness

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	peerMeshRangeMu  sync.Mutex
	peerMeshRangeIdx int
)

// Server is a knot server subprocess booted for testing, with its own
// badgerdb data dir and dynamically allocated ports.
type Server struct {
	Name string
	Config *Config

	HTTPPort int
	AgentPort int

	// BaseURL is the server URL for host-side clients (apiclient).
	BaseURL string
	// ContainerBaseURL is the same server as reached from inside containers
	// (used for --url so spaces can fetch the agent and dial the agent link).
	ContainerBaseURL string

	DataDir string
	LogPath string

	cmd     *exec.Cmd
	logFile *os.File
	mu      sync.Mutex
	stopped bool

	// ExtraArgs are appended to the command line, set before Start.
	ExtraArgs []string

	// OnStart runs after the health check passes (e.g. provisioning).
	bins *Binaries
}

// StartServer boots a fresh server. name is used for logs and identification.
func StartServer(cfg *Config, bins *Binaries, name string, extraArgs ...string) (*Server, error) {
	return StartServerAt(cfg, bins, name, "", extraArgs...)
}

// StartServerAt boots a server whose advertised URL/agent-endpoint use the
// given host address instead of the container host. This is needed when the
// host routing matters (e.g. --wildcard-domain servers, where the domain mux
// only serves the URL host and host.docker.internal is not host-resolvable):
// pass the host's LAN IP, reachable from both the host and containers.
func StartServerAt(cfg *Config, bins *Binaries, name, hostAddr string, extraArgs ...string) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil config")
	}
	if bins == nil {
		return nil, fmt.Errorf("nil binaries (call BuildBinaries first)")
	}

	containerHost := cfg.ContainerHost
	if hostAddr != "" {
		containerHost = hostAddr
	}
	if containerHost == "" {
		// Let the server hand out its own IP: the token resolves to the
		// primary physical interface address at startup (same mechanism
		// production configs use), instead of assuming a per-runtime
		// hostname like host.docker.internal.
		containerHost = "${{ host_ip }}"
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		return nil, err
	}

	httpPort, err := FreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate http port: %w", err)
	}
	agentPort, err := FreePort()
	if err != nil {
		return nil, fmt.Errorf("allocate agent port: %w", err)
	}

	dataDir, err := os.MkdirTemp("", "knot-it-"+name+"-")
	if err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	logDir := filepath.Join(repoRoot, buildDir, "logs")
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, name+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("create log file: %w", err)
	}

	encKey := make([]byte, 16)
	rand.Read(encKey)

	srv := &Server{
		Name:             name,
		Config:           cfg,
		HTTPPort:         httpPort,
		AgentPort:        agentPort,
		BaseURL:          fmt.Sprintf("http://%s:%d", baseListenHost(hostAddr), httpPort),
		ContainerBaseURL: fmt.Sprintf("http://%s:%d", containerHost, httpPort),
		DataDir:          dataDir,
		LogPath:          logPath,
		logFile:          logFile,
		bins:             bins,
	}

	// The HTTP listener binds all interfaces when spaces get the host's
	// real IP: containers reach it via the resolved agent-endpoint/URL
	// host, while the harness keeps using 127.0.0.1 (see baseListenHost).
	listenHost := "0.0.0.0"
	if hostAddr != "" {
		listenHost = hostAddr
	}
	args := []string{
		"server",
		"--listen", fmt.Sprintf("%s:%d", listenHost, httpPort),
		"--listen-agent", fmt.Sprintf("0.0.0.0:%d", agentPort),
		"--agent-endpoint", fmt.Sprintf("%s:%d", containerHost, agentPort),
		"--url", srv.ContainerBaseURL,
		"--use-tls=false",
		"--agent-use-tls=false",
		"--badgerdb-enabled",
		"--badgerdb-path", filepath.Join(dataDir, "badger"),
		"--zone", cfg.Zone,
		"--local-container-runtime-pref", cfg.Runtime,
		"--encrypt", hex.EncodeToString(encKey),
	}
	if cfg.Registry() != "" {
		args = append(args, "--base-image-registry", cfg.Registry())
	}
	if cfg.Runtime == "docker" && cfg.DockerHost != "" {
		args = append(args, "--docker-host", cfg.DockerHost)
	}
	if name == "default" {
		// The shared server runs auth rate limiting as part of the journey:
		// the auth suite trips the limit, verifies the block, then clears
		// it via the admin flush API. The block window is long enough that
		// only the explicit flush (or a restart) recovers it.
		args = append(args,
			"--auth-ip-rate-limiting",
			"--auth-rate-limit-attempts", "8",
			"--auth-rate-limit-window", "300",
			"--auth-rate-limit-block", "600",
		)
	}
	args = append(args, extraArgs...)

	// Pro builds enable the peer mesh, whose published host ports (default
	// 30001+) are global across the docker daemon — parallel test servers
	// would collide. Give every non-default server a disjoint published
	// range; the default server keeps the default range (the peermesh
	// suite runs there).
	if ProBuild && name != "default" {
		peerMeshRangeMu.Lock()
		min := 31000 + peerMeshRangeIdx*100
		peerMeshRangeIdx = (peerMeshRangeIdx + 1) % 25
		peerMeshRangeMu.Unlock()
		args = append(args,
			"--peermesh-port-range-min", fmt.Sprintf("%d", min),
			"--peermesh-port-range-max", fmt.Sprintf("%d", min+99),
		)
	}

	srv.cmd = exec.Command(bins.Server, args...)
	// Full config isolation: run from (and with HOME in) the temp data dir so
	// no knot.toml / .knot.toml from the repo, the user's home or ~/.knot
	// leaks into the test server. Flags above are then the entire config.
	srv.cmd.Dir = dataDir
	env := append(os.Environ(), "HOME="+dataDir)
	if cfg.DockerHost != "" {
		// The runtime detector shells out to the docker CLI; with HOME
		// overridden the CLI can't find its context, so pin the endpoint.
		env = append(env, "DOCKER_HOST="+cfg.DockerHost)
	}
	srv.cmd.Env = env
	srv.cmd.Stdout = logFile
	srv.cmd.Stderr = logFile
	if err := srv.cmd.Start(); err != nil {
		return nil, fmt.Errorf("start server: %w", err)
	}

	if err := srv.waitHealthy(90 * time.Second); err != nil {
		srv.Stop()
		return nil, fmt.Errorf("server %s never became healthy: %w\nserver log tail:\n%s", name, err, srv.LogTail(40))
	}

	return srv, nil
}

// Restart stops the server process and boots a new one with the same
// ports, data dir and flags. State in badger (users, tokens, spaces)
// survives; in-memory state (auth rate-limit blocks) is cleared — which is
// exactly what the rate-limit journey stage needs.
func (s *Server) Restart() error {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return fmt.Errorf("server already stopped")
	}
	oldCmd := s.cmd
	if oldCmd != nil && oldCmd.Process != nil {
		oldCmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { oldCmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			oldCmd.Process.Kill()
			<-done
		}
	}
	s.mu.Unlock()

	// Re-exec with the same path, arguments, working dir and environment.
	cmd := exec.Command(oldCmd.Path, oldCmd.Args[1:]...)
	cmd.Dir = oldCmd.Dir
	cmd.Env = oldCmd.Env
	logFile, err := os.OpenFile(s.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("reopen log: %w", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	s.mu.Lock()
	s.cmd = cmd
	if s.logFile != nil {
		s.logFile.Close()
	}
	s.logFile = logFile
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart server: %w", err)
	}
	if err := s.waitHealthy(90 * time.Second); err != nil {
		return fmt.Errorf("server %s never became healthy after restart: %w\nserver log tail:\n%s", s.Name, err, s.LogTail(40))
	}
	return nil
}

// waitHealthy polls until the server answers any HTTP request.
func (s *Server) waitHealthy(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(s.BaseURL + "/health")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("health check failed: %w", lastErr)
}

// Kill stops the server process immediately without any cleanup — the
// data dir and any containers it created are left behind, simulating a
// crashed node.
func (s *Server) Kill() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Kill()
		s.cmd.Wait()
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
}

// LogTail returns the last n lines of the server log for diagnostics.
func (s *Server) LogTail(n int) string {
	data, err := os.ReadFile(s.LogPath)
	if err != nil {
		return fmt.Sprintf("(no log at %s)", s.LogPath)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// Stop terminates the server and removes its data dir unless Keep is set.
// Servers must be stopped in reverse boot order; tests register cleanup via
// t.Cleanup or TestMain.
func (s *Server) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		s.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() {
			s.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			s.cmd.Process.Kill()
			<-done
		}
	}
	if s.logFile != nil {
		s.logFile.Close()
	}
	if !s.Config.Keep {
		os.RemoveAll(s.DataDir)
	}
}

// ImageSource records where the space image comes from: "cache" when pulled
// through the configured registry cache, or a docker.io note when the cache
// was unreachable from the docker VM.
var ImageSource = "none"

// EnsureImageAvailable makes the space image pullable by the docker daemon
// the server uses. It prefers the configured cache; when the cache is
// unreachable from the docker VM (e.g. a split-DNS LAN cache reachable from
// the host but not from the VM), it switches the whole configuration to pull
// from Docker Hub directly by clearing the cache prefix, so templates'
// ${{ base_image_registry }} references resolve to docker.io.
func EnsureImageAvailable(cfg *Config, refs ...string) error {
	if cfg.Runtime != "docker" {
		return nil
	}
	for _, ref := range refs {
		if imageExistsLocally(ref) && cfg.DockerCache == "" {
			continue
		}
		err := dockerPull(ref, 20*time.Minute)
		if err == nil {
			ImageSource = "cache"
			continue
		}
		fmt.Fprintf(os.Stderr, "warning: cache pull failed for %s (%v), falling back to Docker Hub\n", ref, err)

		if cfg.DockerCache == "" {
			return fmt.Errorf("docker pull %s: %w", ref, err)
		}
		fallback := strings.TrimPrefix(ref, strings.TrimSuffix(cfg.DockerCache, "/")+"/")
		if fallback == ref {
			return fmt.Errorf("docker pull %s: %w", ref, err)
		}
		if ferr := dockerPull(fallback, 20*time.Minute); ferr != nil {
			return fmt.Errorf("docker pull %s via cache: %w / direct: %w", ref, err, ferr)
		}
		// Switch the configuration (and all later template rendering) to
		// the direct registry so the daemon can pull spaces' images.
		cfg.DockerCache = ""
		ImageSource = "docker.io (cache unreachable from docker VM)"
	}
	return nil
}

func imageExistsLocally(ref string) bool {
	return exec.Command("docker", "image", "inspect", ref).Run() == nil
}

func dockerPull(ref string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "pull", ref)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timeout")
	}
	if err != nil {
		return fmt.Errorf("%s", truncate(string(out), 300))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// PruneTestContainers force-removes any containers left behind by the run
// (safety net when server-side deletion failed). Only docker is supported.
func PruneTestContainers(namePrefix string) {
	out, err := exec.Command("docker", "ps", "-a", "--filter", "name="+namePrefix,
		"--format", "{{.ID}}").Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.Command("docker", "rm", "-f", id).Run()
	}
}

// PruneTestContainersByImage removes leftover containers running a given
// image, regardless of name.
func PruneTestContainersByImage(imageRef string) {
	if imageRef == "" {
		return
	}
	out, err := exec.Command("docker", "ps", "-a", "--filter", "ancestor="+imageRef,
		"--format", "{{.ID}}").Output()
	if err != nil {
		return
	}
	for _, id := range strings.Fields(string(out)) {
		exec.Command("docker", "rm", "-f", id).Run()
	}
}

// RunHelperContainer starts a detached helper container (e.g. vault dev,
// victoria-logs) from the given image through the cache. Ports are published
// on 127.0.0.1 as hostPort:containerPort. It returns the container id; the
// container is removed with RemoveContainer.
func RunHelperContainer(cfg *Config, name string, hostPort int, image string, args ...string) (string, error) {
	full := cfg.CacheImageRef(image)
	cmdArgs := []string{"run", "-d", "--name", name,
		"-p", fmt.Sprintf("127.0.0.1:%d:%d", hostPort, hostPort)}
	cmdArgs = append(cmdArgs, full)
	cmdArgs = append(cmdArgs, args...)
	out, err := exec.Command("docker", cmdArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("docker run %s: %w\n%s", full, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// RemoveContainer force-removes a helper container.
func RemoveContainer(idOrName string) {
	exec.Command("docker", "rm", "-f", idOrName).Run()
}

// baseListenHost picks the host clients should use to reach the server:
// the loopback normally, or the LAN address for wildcard-domain servers
// whose host routing matches the advertised URL host.
func baseListenHost(hostAddr string) string {
	if hostAddr == "" {
		return "127.0.0.1"
	}
	return hostAddr
}

// LanIP returns the host's outbound LAN address, reachable from both the
// host itself and from containers on the default bridge network.
func LanIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}
