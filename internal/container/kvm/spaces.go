package kvm

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util"
	"gopkg.in/yaml.v3"
)

const (
	spaceStartupTimeout = 30 * time.Minute
	shutdownGrace       = 90 * time.Second
	pollInterval        = 2 * time.Second
)

// ---- job spec (parsed from template YAML) ----

type jobSpec struct {
	// Name is the libvirt domain name. Defaults to
	// <username>-<spacename>, matching the container platforms.
	Name string `yaml:"name,omitempty"`
	// Hostname defaults to the space name.
	Hostname string `yaml:"hostname,omitempty"`
	// Image is a cloud-init capable qcow2: a bare name resolved against
	// the node's cloud image library, a URL knot downloads into its cache,
	// or a path on the node's filesystem. It must match the node's
	// architecture.
	Image string `yaml:"image"`
	// Memory, e.g. 2G. Defaults to the base image being booted as-is when
	// unset (virt-install requires an explicit value, so the fallback is
	// 512M).
	Memory string `yaml:"memory,omitempty"`
	// CPUs is the vCPU count. Defaults to 1.
	CPUs string `yaml:"cpus,omitempty"`
	// Disk caps the overlay's virtual size, e.g. 20G. Empty keeps the base
	// image's size.
	Disk string `yaml:"disk,omitempty"`
	// Environment entries are written into the agent's environment file.
	Environment []string `yaml:"environment,omitempty"`
	// Devices are host devices passed through to the VM, in virt-install
	// hostdev form: PCI addresses (pci_0000_01_00_0), USB bus.device
	// (usb_002_003) or vendor:product hex pairs (0x8086:0x1234). The host
	// must have the device bound to vfio (and IOMMU enabled) for PCI.
	Devices []string `yaml:"devices,omitempty"`
}

// ---- ContainerManager implementation ----

func (c *KVMClient) CreateSpaceJob(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	job, err := model.ResolveVariables(template.Job, template, space, user, variables)
	if err != nil {
		return err
	}

	var spec jobSpec
	if err = yaml.Unmarshal([]byte(job), &spec); err != nil {
		return err
	}

	if spec.Image == "" {
		return fmt.Errorf("image must be set")
	}
	if template.IsKvmBridged() && space.IPAddress == "" {
		return fmt.Errorf("space has no IP address assigned")
	}

	domain := spec.Name
	if domain == "" {
		domain = fmt.Sprintf("%s-%s", user.Username, space.Name)
	}
	domain = sanitizeDomainName(domain)

	// Per-space directories sit directly under the images path
	// (<images>/<domain>), so reserve the names the images path itself uses.
	if domain == "base" || domain == "cloud-images" {
		return fmt.Errorf("domain name %q collides with a reserved directory in the KVM images path", domain)
	}

	// Bridged spaces carry a static IP resolved into a network plan; NAT
	// spaces DHCP from the libvirt network and need no plan.
	var plan *networkPlan
	if template.IsKvmBridged() {
		plan, err = resolveNetwork(template, space)
		if err != nil {
			return err
		}
	}

	memoryMiB := uint64(512)
	if spec.Memory != "" {
		memBytes, err := util.ConvertToBytes(spec.Memory)
		if err != nil {
			return fmt.Errorf("invalid memory value %q (expected e.g. 512M, 4G): %w", spec.Memory, err)
		}
		memoryMiB = uint64(memBytes / (1024 * 1024))
		if memoryMiB == 0 {
			memoryMiB = 512
		}
	}

	vcpus := 1
	if spec.CPUs != "" {
		vcpus, err = strconv.Atoi(strings.TrimSpace(strings.Split(spec.CPUs, ".")[0]))
		if err != nil || vcpus < 1 {
			return fmt.Errorf("invalid cpus value %q (expected a positive integer)", spec.CPUs)
		}
	}

	// The hash the space last deployed with, captured before it is
	// overwritten below — a shut-off VM is only redefined when the spec
	// changed, otherwise it is simply booted again.
	previousHash := space.TemplateHash

	// Record deploying
	db := database.GetInstance()
	cfg := config.GetServerConfig()
	space.IsPending = true
	space.IsDeployed = false
	space.IsDeleting = false
	space.TemplateHash = template.Hash
	space.Zone = cfg.Zone
	space.StartedAt = time.Now().UTC()
	space.UpdatedAt = hlc.Now()
	if err = db.SaveSpace(space, []string{"IsPending", "IsDeployed", "IsDeleting", "TemplateHash", "Zone", "UpdatedAt", "StartedAt"}); err != nil {
		c.logger.Error("creating space job error", "space_id", space.Id)
		return err
	}

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), spaceStartupTimeout)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.logger.Error("creating space job error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		// Each space's disk overlay, cloud-init seed and files live in
		// <images>/<domain> — flat under the images path, no "spaces"
		// level (the domain is <username>-<spacename> by default, so the
		// directory is self-describing).
		spaceDir := filepath.Join(imagesDir(), domain)
		diskPath := filepath.Join(spaceDir, "disk.qcow2")

		state, err := c.domainState(ctx, domain)
		if err != nil {
			c.logger.Error("checking existing domain error", "domain", domain, "error", err)
			return
		}
		if state == "running" {
			c.logger.Error("domain already exists and is running", "domain", domain)
			return
		}

		// The network attachment must resolve before anything slow happens
		// (image downloads, disk creation) — a name that is neither a host
		// bridge nor a libvirt network fails the boot obscurely, or worse
		// falls back somewhere the space's static IP is unreachable. NAT
		// templates attach straight to their libvirt network.
		var attach *attachment
		if template.IsKvmNat() {
			name := template.KvmBridge
			if name == "" {
				name = "default"
			}
			attach = &attachment{Mode: "network", Name: name}
		} else {
			attach, err = resolveAttachment(template.KvmBridge)
			if err != nil {
				c.logger.Error("resolving network attachment error", "bridge", template.KvmBridge, "error", err)
				return
			}
		}
		c.ensureAttachmentActive(ctx, attach)

		needsDefine := false

		if state == "" {
			// First boot — or a define that failed after the domain was
			// undefined (a redefine interrupted by a virt-install error).
			// Either way, a disk that is already present is the space's
			// machine: recreating the overlay would wipe it.
			needsDefine = true
			if err := c.ensureSpaceDisk(ctx, spec.Image, spec.Disk, domain, spaceDir, diskPath); err != nil {
				c.logger.Error("ensuring space disk error", "domain", domain, "error", err)
				return
			}
		} else if previousHash != "" && previousHash != template.Hash {
			// The VM already exists (stopped) but the template spec
			// changed: redefine the domain over the existing disk with a
			// fresh cloud-init seed so new resources and agent environment
			// take effect — the disk and everything on it persist.
			needsDefine = true
			c.logger.Info("template changed since last deploy, redefining stopped domain", "domain", domain, "previous_hash", previousHash, "new_hash", template.Hash)
			if _, err := c.runVirsh(ctx, "undefine", domain); err != nil && !virshErrorIsNotFound(err) {
				c.logger.Error("undefining existing domain error", "domain", domain, "error", err)
				return
			}
		} else {
			// The VM exists and the spec is unchanged: a stop/start cycle.
			// The seed is regenerated in place (same file the domain's
			// CDROM already points at) so an IP edited while the space was
			// stopped reaches the machine — cloud-init's instance-id
			// derives from the seed content, so a changed network is
			// re-applied on this boot and an unchanged one boots fast.
			c.logger.Info("starting existing domain", "domain", domain)

			if err := os.MkdirAll(filepath.Join(spaceDir, "cloud-init"), 0755); err != nil {
				c.logger.Error("creating space directory error", "dir", spaceDir, "error", err)
				return
			}
			if _, err := c.writeCloudInitSeed(ctx, filepath.Join(spaceDir, "cloud-init"), buildCloudInit(template, space, user, &spec, plan)); err != nil {
				c.logger.Error("writing cloud-init seed error", "domain", domain, "error", err)
				return
			}

			startCmd := "start"
			if state == "paused" {
				startCmd = "resume"
			}
			if _, err := c.runVirsh(ctx, startCmd, domain); err != nil {
				c.logger.Error("starting existing domain error", "domain", domain, "error", err)
				return
			}
		}

		if needsDefine {
			// First boot or redefinition: fresh seed, then virt-install
			// (over the existing disk when redefining).
			seed, err := c.writeCloudInitSeed(ctx, filepath.Join(spaceDir, "cloud-init"), buildCloudInit(template, space, user, &spec, plan))
			if err != nil {
				c.logger.Error("writing cloud-init seed error", "domain", domain, "error", err)
				return
			}

			installArgs := []string{
				"--connect", libvirtURI,
				"--name", domain,
				"--memory", strconv.FormatUint(memoryMiB, 10),
				"--vcpus", strconv.Itoa(vcpus),
				"--disk", "path=" + diskPath + ",format=qcow2,bus=virtio",
				"--disk", "path=" + seed + ",device=cdrom,readonly=on",
				"--network", attach.arg(),
				// The guest-agent channel is what qemu-guest-agent (installed
				// by cloud-init) talks over; without it 'virsh domifaddr
				// --source agent' can never see the VM's static address.
				"--channel", "unix,target_type=virtio,name=org.qemu.guest_agent.0",
				// Explicit localhost VNC: the framebuffer QEMU renders is the
				// display knot's VNC bridge proxies to the browser (the
				// listen address is pinned so the only door is the node
				// itself). virtio-gpu is the video device: modern guests
				// kernel-drive it (a bare VGA/cirrus device often leaves the
				// cloud image with no framebuffer console, i.e. a blank
				// display).
				"--graphics", "vnc,listen=127.0.0.1",
				"--video", "virtio",
				"--os-variant", "generic",
				"--import",
				"--noautoconsole",
			}
			for _, device := range spec.Devices {
				installArgs = append(installArgs, "--hostdev", device)
			}
			c.logger.Debug("starting domain", "domain", domain, "memory_mib", memoryMiB, "vcpus", vcpus, "network", attach.arg(), "hostdevs", len(spec.Devices))
			if _, err := c.runCommand(ctx, "virt-install", installArgs...); err != nil {
				c.logger.Error("starting domain error", "domain", domain, "error", err)
				return
			}
		}

		if _, err := c.waitForState(ctx, domain, "running", 10*time.Minute, pollInterval); err != nil {
			c.logger.Error("waiting for domain error", "domain", domain, "error", err)
			return
		}

		c.logger.Debug("domain running", "domain", domain)

		// Record what the NICs actually attached to — the bridge source is
		// the first thing to check when a VM's static IP is unreachable —
		// and the display URI, for attaching a console viewer when a VM
		// boots but its agent never connects.
		if ifaces, err := c.runVirsh(ctx, "domiflist", domain); err == nil {
			c.logger.Debug("domain interfaces", "domain", domain, "ifaces", ifaces)
		}
		if display, err := c.runVirsh(ctx, "domdisplay", domain); err == nil && display != "" {
			c.logger.Info("domain console available", "domain", domain, "display", display)
		}

		oldSpace := *space
		space.ContainerId = domain
		space.IsPending = false
		space.IsDeployed = true
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"ContainerId", "IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
			c.logger.Error("creating space job error", "space_id", space.Id)
			return
		}
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		succeeded = true
		sse.PublishSpaceChanged(space.Id, space.UserId)
		service.CheckSpaceLifecycleEvents(&oldSpace, space)
	}()

	return nil
}

func (c *KVMClient) DeleteSpaceJob(space *model.Space, onStopped func()) error {
	c.logger.Debug("deleting space job", "space_id", space.Id, "domain", space.ContainerId)

	db := database.GetInstance()

	space.IsPending = true
	space.UpdatedAt = hlc.Now()
	if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
		return err
	}

	if transport := service.GetTransport(); transport != nil {
		transport.GossipSpace(space)
	}
	sse.PublishSpaceChanged(space.Id, space.UserId)

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()

		succeeded := false
		defer func() {
			space.IsPending = false
			space.UpdatedAt = hlc.Now()
			if err := db.SaveSpace(space, []string{"IsPending", "UpdatedAt"}); err != nil {
				c.logger.Error("deleting space job error", "space_id", space.Id)
			}
			if transport := service.GetTransport(); transport != nil {
				transport.GossipSpace(space)
			}
			if !succeeded {
				sse.PublishSpaceChanged(space.Id, space.UserId)
			}
		}()

		// Stopping a KVM space shuts the VM down but keeps it — the
		// domain stays defined and the disk untouched, so the next start
		// boots the same machine. Only deleting the space destroys it.
		if err := c.shutdownDomain(ctx, space.ContainerId); err != nil {
			c.logger.Error("shutting down domain error", "domain", space.ContainerId, "error", err)
			return
		}

		oldSpace := *space
		space.IsPending = false
		space.IsDeployed = false
		space.UpdatedAt = hlc.Now()
		if err := db.SaveSpace(space, []string{"IsPending", "IsDeployed", "UpdatedAt"}); err != nil {
			c.logger.Error("deleting space job error", "space_id", space.Id)
			return
		}
		if transport := service.GetTransport(); transport != nil {
			transport.GossipSpace(space)
		}
		succeeded = true
		sse.PublishSpaceChanged(space.Id, space.UserId)
		service.CheckSpaceLifecycleEvents(&oldSpace, space)

		if onStopped != nil {
			onStopped()
		}
	}()

	return nil
}

// shutdownDomain shuts a running domain down — gracefully first, forcefully
// after the grace period — and leaves it defined with its storage intact:
// stop/start cycles keep the VM, only destroying the space removes it.
func (c *KVMClient) shutdownDomain(ctx context.Context, domain string) error {
	if domain == "" {
		return nil
	}

	state, err := c.domainState(ctx, domain)
	if err != nil {
		return err
	}
	if state == "" || state == "shut off" {
		return nil
	}

	if state == "running" {
		c.logger.Debug("shutting down domain", "domain", domain)
		if _, err := c.runVirsh(ctx, "shutdown", domain); err != nil {
			c.logger.Error("shutting down domain error", "domain", domain, "error", err)
		}

		if last, waitErr := c.waitForState(ctx, domain, "shut off", shutdownGrace, pollInterval); waitErr != nil {
			c.logger.Warn("graceful shutdown did not complete, destroying domain", "domain", domain, "state", last)
			if _, err := c.runVirsh(ctx, "destroy", domain); err != nil {
				return err
			}
		}
		return nil
	}

	// Paused / pmsuspended / anything else: shutdown won't land, force it.
	c.logger.Warn("domain in unexpected state, destroying", "domain", domain, "state", state)
	_, err = c.runVirsh(ctx, "destroy", domain)
	return err
}

// destroyDomain performs the full teardown: shut down (forcefully if
// needed), undefine, and remove the space's disk, seed ISO and cloud-init
// files. Called when a space is deleted or its artifacts are cleaned up
// after a node migration.
func (c *KVMClient) destroyDomain(ctx context.Context, domain string) error {
	if domain == "" {
		return nil
	}

	if err := c.shutdownDomain(ctx, domain); err != nil {
		return err
	}

	if _, err := c.runVirsh(ctx, "undefine", domain); err != nil && !virshErrorIsNotFound(err) {
		return err
	}

	// Remove the space's disk overlay, seed ISO and cloud-init files. The
	// domain name is sanitized on create so it cannot traverse paths.
	spaceDir := filepath.Join(imagesDir(), sanitizeDomainName(domain))
	if err := os.RemoveAll(spaceDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	return nil
}

func (c *KVMClient) CreateSpaceVolumes(user *model.User, template *model.Template, space *model.Space, variables map[string]interface{}) error {
	// KVM spaces carry no managed volumes: the VM's disk is created with the
	// job and removed with the domain. Volume specs are rejected at template
	// validation time.
	return nil
}

// DeleteSpaceVolumes destroys the space's VM entirely — undefine the domain
// and remove its disk, seed and cloud-init files. This runs when a space is
// deleted; a stop keeps everything (see shutdownDomain).
func (c *KVMClient) DeleteSpaceVolumes(space *model.Space) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := c.destroyDomain(ctx, space.ContainerId); err != nil {
		return err
	}
	return nil
}

func (c *KVMClient) CleanupSpaceArtifacts(space *model.Space) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return c.destroyDomain(ctx, space.ContainerId)
}

func (c *KVMClient) ListRunningSpaceRuntimeRefs() (map[string]bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// --state-running, not --all: with persistent VMs a shut-off domain is
	// the normal state of a stopped space, not a live runtime.
	out, err := c.runVirsh(ctx, "list", "--state-running", "--name")
	if err != nil {
		return nil, err
	}

	refs := make(map[string]bool)
	for _, name := range strings.Split(out, "\n") {
		if name = strings.TrimSpace(name); name != "" {
			refs[name] = true
		}
	}
	return refs, nil
}

// StopSpaceRuntime stops an orphaned running VM without destroying it: the
// space still exists, so the domain and its disk must survive.
func (c *KVMClient) StopSpaceRuntime(space *model.Space) error {
	if space.ContainerId == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	return c.shutdownDomain(ctx, space.ContainerId)
}

func (c *KVMClient) CreateVolume(vol *model.Volume, variables map[string]interface{}) error {
	return fmt.Errorf("volumes are not supported for the kvm platform")
}

func (c *KVMClient) DeleteVolume(vol *model.Volume, variables map[string]interface{}) error {
	return fmt.Errorf("volumes are not supported for the kvm platform")
}
