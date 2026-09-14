package kvm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// Libvirt URI the backend talks to. KVM spaces always target the local
// system instance — the owning node is chosen at space create time.
const libvirtURI = "qemu:///system"

// downloadLocks serialises base image downloads per destination file:
// without it, two spaces first booting the same image URL write the same
// .part file concurrently and corrupt the cached base under both VMs.
var downloadLocks sync.Map

type KVMClient struct {
	logger logger.Logger
}

func NewClient() *KVMClient {
	return &KVMClient{
		logger: log.WithGroup("kvm"),
	}
}

// ---- command helpers ----

// runVirsh executes virsh against the local libvirt daemon and returns
// trimmed stdout. A non-zero exit surfaces stderr in the error.
func (c *KVMClient) runVirsh(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"--connect", libvirtURI}, args...)
	return c.runCommand(ctx, "virsh", full...)
}

func (c *KVMClient) runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// virshErrorIsNotFound reports whether a virsh error is the "domain does not
// exist" case, which every caller treats as already-done rather than failure.
func virshErrorIsNotFound(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "failed to get domain"))
}

// domainState returns the domain's state string (e.g. "running", "shut off")
// or "" when the domain does not exist.
func (c *KVMClient) domainState(ctx context.Context, name string) (string, error) {
	out, err := c.runVirsh(ctx, "domstate", name)
	if err != nil {
		if virshErrorIsNotFound(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(strings.SplitN(out, "\n", 2)[0]), nil
}

// DomainRunning reports whether the named domain exists and is running —
// the precondition for attaching its serial console.
func (c *KVMClient) DomainRunning(ctx context.Context, domain string) (bool, error) {
	state, err := c.domainState(ctx, domain)
	if err != nil {
		return false, err
	}
	return state == "running", nil
}

// ---- image management ----

// imagesDir returns the configured base directory for KVM images as an
// absolute path. Everything derived from it (disk overlays, cloud-init
// seeds, downloaded bases) is handed to qemu-img and virt-install, which
// resolve relative paths against their own file locations — a relative
// backing file silently points into the overlay's directory.
func imagesDir() string {
	cfg := config.GetServerConfig()
	images := "/var/lib/libvirt/images/knot"
	if cfg != nil && cfg.KVM.ImagesPath != "" {
		images = cfg.KVM.ImagesPath
	}
	if abs, err := filepath.Abs(images); err == nil {
		return abs
	}
	return images
}

// cloudImagesDir returns the directory bare image names resolve against,
// as an absolute path (see imagesDir for why). Empty config follows the
// images path (<images path>/cloud-images); --kvm-cloud-image-path pins it
// elsewhere.
func cloudImagesDir() string {
	cfg := config.GetServerConfig()
	dir := ""
	if cfg != nil {
		dir = cfg.KVM.CloudImagePath
	}
	if dir == "" {
		dir = filepath.Join(imagesDir(), "cloud-images")
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// baseImagesDir returns the cache directory for URL-downloaded base images,
// as an absolute path (see imagesDir for why). Empty config follows the
// images path (<images path>/base) so relocating the images path moves the
// cache too; --kvm-base-image-path pins it elsewhere.
func baseImagesDir() string {
	cfg := config.GetServerConfig()
	dir := ""
	if cfg != nil {
		dir = cfg.KVM.BaseImagePath
	}
	if dir == "" {
		dir = filepath.Join(imagesDir(), "base")
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs
	}
	return dir
}

// resolveBaseImage returns a local path to the base image. Three forms are
// accepted: an http(s) URL (downloaded into the images cache on first use, so
// nodes sharing a cache directory don't re-download), an absolute or ~/relative
// path, or a bare name that resolves against the node's cloud image library
// (with ".qcow2" appended when the name has no extension).
func (c *KVMClient) resolveBaseImage(ctx context.Context, image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", fmt.Errorf("image must be set")
	}

	if strings.HasPrefix(image, "http://") || strings.HasPrefix(image, "https://") {
		baseDir := baseImagesDir()
		if err := os.MkdirAll(baseDir, 0755); err != nil {
			return "", err
		}

		name := filepath.Base(strings.TrimRight(image, "/"))
		// Strip query strings from URL-derived file names.
		if idx := strings.Index(name, "?"); idx != -1 {
			name = name[:idx]
		}
		if name == "" || name == "." || name == "/" {
			return "", fmt.Errorf("cannot derive a file name from image URL %q", image)
		}

		dest := filepath.Join(baseDir, name)

		lockAny, _ := downloadLocks.LoadOrStore(dest, &sync.Mutex{})
		lock := lockAny.(*sync.Mutex)
		lock.Lock()
		defer lock.Unlock()

		if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
			return dest, nil
		}

		c.logger.Info("downloading KVM base image", "url", image, "dest", dest)
		if err := c.downloadFile(ctx, image, dest); err != nil {
			return "", err
		}
		return dest, nil
	}

	if filepath.IsAbs(image) || strings.HasPrefix(image, "~/") || strings.HasPrefix(image, "~") {
		if _, err := os.Stat(image); err != nil {
			return "", fmt.Errorf("base image %q not accessible: %w", image, err)
		}
		// qemu-img resolves backing files against the overlay's
		// directory, so relative paths must be made absolute here.
		if abs, err := filepath.Abs(image); err == nil {
			return abs, nil
		}
		return image, nil
	}

	// Bare name: resolve against the cloud image library. The name as given
	// wins; a name not ending in a disk image suffix also tries <name>.qcow2
	// (so "ubuntu-24.04" finds ubuntu-24.04.qcow2 — version-numbered names
	// look like extensions to filepath.Ext).
	dir := cloudImagesDir()
	candidates := []string{filepath.Join(dir, image)}
	if !strings.HasSuffix(image, ".qcow2") && !strings.HasSuffix(image, ".img") {
		candidates = append(candidates, filepath.Join(dir, image+".qcow2"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no cloud image %q in %s (tried %s) — place cloud images there or set --kvm-cloud-image-path, or use an absolute path or URL", image, dir, strings.Join(candidates, ", "))
}

func (c *KVMClient) downloadFile(ctx context.Context, url, dest string) error {
	part := dest + ".part"
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("downloading %s: unexpected status %d", url, resp.StatusCode)
	}

	f, err := os.Create(part)
	if err != nil {
		return err
	}

	written, err := io.Copy(f, resp.Body)
	closeErr := f.Close()
	if err != nil {
		os.Remove(part)
		return fmt.Errorf("downloading %s: %w", url, err)
	}
	if closeErr != nil {
		os.Remove(part)
		return closeErr
	}
	if written == 0 {
		os.Remove(part)
		return fmt.Errorf("downloading %s: response was empty", url)
	}

	return os.Rename(part, dest)
}

// createOverlay makes the space's copy-on-write disk over the base image.
// size may be empty, in which case the overlay inherits the base image's
// virtual size.
func (c *KVMClient) createOverlay(ctx context.Context, base, overlay, size string) error {
	args := []string{"create", "-f", "qcow2", "-o", "backing_file=" + base + ",backing_fmt=qcow2", overlay}
	if size != "" {
		args = append(args, size)
	}
	_, err := c.runCommand(ctx, "qemu-img", args...)
	return err
}

// ensureSpaceDisk makes sure the space's disk exists, creating the overlay on
// first boot. A disk that is already present is kept as-is: it is the space's
// machine — typically left behind by a define that failed after the domain
// was undefined (an interrupted redefine), and recreating the overlay would
// destroy the data on it. The space directory is created with a hint for the
// common permission problem on the images path.
func (c *KVMClient) ensureSpaceDisk(ctx context.Context, image, diskSize, domain, spaceDir, diskPath string) error {
	if _, err := os.Stat(diskPath); err == nil {
		c.logger.Info("domain missing but disk exists, defining over existing disk", "domain", domain)
		return nil
	}

	base, err := c.resolveBaseImage(ctx, image)
	if err != nil {
		return fmt.Errorf("resolving base image: %w", err)
	}

	if err := os.MkdirAll(spaceDir, 0755); err != nil {
		return fmt.Errorf("creating space directory %s: %w — the KVM images path must be writable by the knot user and traversable by the qemu user; pre-create it with e.g. 'mkdir -p /var/lib/libvirt/images/knot && chown <knot-user> /var/lib/libvirt/images/knot'", spaceDir, err)
	}
	if err := c.createOverlay(ctx, base, diskPath, diskSize); err != nil {
		return fmt.Errorf("creating disk overlay: %w", err)
	}
	return nil
}

// bridgeExists reports whether name is an existing Linux bridge on the host.
// A missing bridge must fail the boot loudly: virt-install/libvirt may fall
// back to the default NAT network (virbr0), where the space's static IP is
// unreachable from the cluster network.
func bridgeExists(name string) bool {
	if name == "" {
		return false
	}
	bridgeDir := filepath.Join("/sys/class/net", name, "bridge")
	info, err := os.Stat(bridgeDir)
	return err == nil && info.IsDir()
}

// attachment is how a VM's NIC connects to the outside world: directly to a
// host Linux bridge (bridge mode) or through a libvirt virtual network
// (network mode — e.g. "default", the built-in NAT network). Both put the
// NIC on the same L2 segment the network runs on, so the guest's cloud-init
// static IP works either way.
type attachment struct {
	Mode string // "bridge" or "network"
	Name string
}

// arg renders the attachment as a virt-install --network option value.
func (a *attachment) arg() string {
	return a.Mode + "=" + a.Name + ",model=virtio"
}

// resolveAttachment resolves the spec's network.bridge value in bridged
// mode: it must name a host Linux bridge the VM's tap attaches to directly.
// (libvirt networks belong to nat mode — a static IP behind a NAT network
// is unreachable, so we don't allow the mix.)
func resolveAttachment(value string) (*attachment, error) {
	if value == "" {
		value = "br0"
	}

	if bridgeExists(value) {
		return &attachment{Mode: "bridge", Name: value}, nil
	}

	return nil, fmt.Errorf("bridge %q does not exist on this node — create it (e.g. 'ip link add %s type bridge' and enslave the physical NIC) or switch the template to mode: nat for a libvirt network", value, value)
}

// ensureAttachmentActive starts an inactive libvirt network so the domain can
// attach to it. Host bridges need no such step.
func (c *KVMClient) ensureAttachmentActive(ctx context.Context, a *attachment) {
	if a.Mode != "network" {
		return
	}
	if active, err := c.runVirsh(ctx, "net-list", "--name"); err == nil {
		for _, name := range strings.Split(active, "\n") {
			if strings.TrimSpace(name) == a.Name {
				return // already active
			}
		}
	}
	c.logger.Info("starting inactive libvirt network", "network", a.Name)
	if _, err := c.runVirsh(ctx, "net-start", a.Name); err != nil {
		c.logger.Warn("failed to start libvirt network", "network", a.Name, "error", err)
	}
}

// sanitizeDomainName reduces a resolved name to the character set libvirt
// accepts for domain names (letters, digits, '_', '.', '+', '-').
func sanitizeDomainName(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '_' || r == '.' || r == '+' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "space"
	}
	return out
}

// waitForState polls the domain's state until it matches want or the timeout
// elapses. Returns the last observed state.
func (c *KVMClient) waitForState(ctx context.Context, name, want string, timeout time.Duration, interval time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for {
		state, err := c.domainState(ctx, name)
		if err != nil {
			return "", err
		}
		if state == want {
			return state, nil
		}
		if state == "" {
			return "", fmt.Errorf("domain %s disappeared while waiting for state %q", name, want)
		}
		if time.Now().After(deadline) {
			return state, fmt.Errorf("timeout waiting for domain %s to reach state %q (currently %q)", name, want, state)
		}
		select {
		case <-ctx.Done():
			return state, ctx.Err()
		case <-time.After(interval):
		}
	}
}
