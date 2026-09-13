package kvm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/container"
	"github.com/paularlott/knot/internal/database/model"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// cloudInitData is the NoCloud seed content for a space's VM.
type cloudInitData struct {
	UserData      string
	NetworkConfig string
	MetaData      string
}

// networkPlan is the resolved static network configuration handed to the VM.
type networkPlan struct {
	IP        net.IP
	PrefixLen int
	Gateway   net.IP
	DNS       []net.IP
}

// resolveNetwork derives the static address, gateway and resolvers from the
// template's KVM network fields and the space's chosen IP. The gateway
// defaults to the network's first usable address; DNS falls back to the
// gateway plus public resolvers so the agent fetch works even when the
// gateway serves no DNS.
func resolveNetwork(template *model.Template, space *model.Space) (*networkPlan, error) {
	_, ipNet, err := net.ParseCIDR(strings.TrimSpace(template.KvmNetworkCidr))
	if err != nil {
		return nil, fmt.Errorf("template KVM network CIDR %q is invalid: %w", template.KvmNetworkCidr, err)
	}

	ip := net.ParseIP(strings.TrimSpace(space.IPAddress))
	if ip == nil {
		return nil, fmt.Errorf("space IP address %q is invalid", space.IPAddress)
	}
	if !ipNet.Contains(ip) {
		return nil, fmt.Errorf("space IP address %s is outside the template network %s", ip, ipNet)
	}

	prefixLen, _ := ipNet.Mask.Size()

	gateway := net.ParseIP(strings.TrimSpace(template.KvmGateway))
	if gateway == nil {
		// First usable address of the network (network base + 1) — the
		// conventional bridge IP for a small deployment.
		gateway = nextIP(ipNet.IP)
	}
	if !ipNet.Contains(gateway) {
		return nil, fmt.Errorf("gateway %s is outside the template network %s", gateway, ipNet)
	}

	dns, err := kvmResolvers()
	if err != nil {
		return nil, err
	}

	return &networkPlan{IP: ip, PrefixLen: prefixLen, Gateway: gateway, DNS: dns}, nil
}

// kvmResolvers returns the DNS servers bridged VMs use, from the server's
// KVM config. Empty falls back to Cloudflare's pair (covering programmatic
// and test configs); the gateway is never injected implicitly.
func kvmResolvers() ([]net.IP, error) {
	cfg := config.GetServerConfig()
	raw := []string{"1.1.1.1", "1.0.0.1"}
	if cfg != nil && len(cfg.KVM.Resolvers) > 0 {
		raw = cfg.KVM.Resolvers
	}

	var resolvers []net.IP
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		ip := net.ParseIP(entry)
		if ip == nil || ip.To4() == nil {
			return nil, fmt.Errorf("invalid KVM resolver %q (expected an IPv4 address)", entry)
		}
		resolvers = append(resolvers, ip)
	}
	if len(resolvers) == 0 {
		return nil, fmt.Errorf("no valid KVM resolvers configured")
	}
	return resolvers, nil
}

func nextIP(ip net.IP) net.IP {
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			break
		}
	}
	return next
}

// buildCloudInit renders the NoCloud user-data, network-config and meta-data
// files. The user-data creates the OS user and installs a knot-agent systemd
// unit that fetches the agent binary from the server on first boot; the
// network-config applies the space's static IP.
func buildCloudInit(template *model.Template, space *model.Space, user *model.User, spec *jobSpec, plan *networkPlan) *cloudInitData {
	cfg := config.GetServerConfig()

	// The OS account is always the space owner's knot username — sharing
	// works because sessions ride the agent, and the account tracks the
	// owner rather than being overridden per template.
	osUser := user.Username
	if osUser == "" {
		osUser = "knot"
	}
	hostname := spec.Hostname
	if hostname == "" {
		hostname = space.Name
	}

	timezone := user.Timezone
	if timezone == "" {
		timezone = cfg.Timezone
	}

	// The agent's environment: the server bootstrap vars knot injects for
	// every runtime, the template's port map, and the spec's own entries.
	// Every value is single-quoted when written to agent.env: the installer
	// script sources the file with /bin/sh, where an unquoted value with
	// spaces parses as a command ("TEST=Testing env 1" runs `env 1`), and
	// single quotes are also valid for systemd's EnvironmentFile. Values
	// are literal — no shell expansion, which is the correct env-file
	// semantics.
	env := []string{
		"KNOT_SERVER=" + strings.TrimSuffix(cfg.URL, "/"),
		"KNOT_AGENT_ENDPOINT=" + cfg.AgentEndpoint,
		"KNOT_SPACEID=" + space.Id,
	}
	env = append(env, container.AgentRegistrationEnv(cfg, space.Id)...)
	env = append(env, container.BuildPortEnvVars(template)...)
	env = append(env, spec.Environment...)
	if user.ServicePassword != "" {
		env = append(env, "KNOT_SERVICE_PASSWORD="+user.ServicePassword)
	}
	for i, entry := range env {
		env[i] = quoteEnvEntry(entry)
	}

	type userData struct {
		Hostname   string                   `yaml:"hostname"`
		Timezone   string                   `yaml:"timezone,omitempty"`
		ManageEtc  bool                     `yaml:"manage_etc_hosts"`
		SSHPwAuth  bool                     `yaml:"ssh_pwauth"`
		Users      []map[string]interface{} `yaml:"users"`
		WriteFiles []map[string]interface{} `yaml:"write_files"`
		Packages   []string                 `yaml:"packages,omitempty"`
		// Explicitly pinned so the image's cloud.cfg can't turn a deploy
		// into a dist-upgrade — no index refresh, no upgrades; only the
		// listed packages are installed from the image's baked indexes.
		PackageUpdate  bool     `yaml:"package_update"`
		PackageUpgrade bool     `yaml:"package_upgrade"`
		RunCmd         []string `yaml:"runcmd"`
	}

	// The OS account's password is the owner's knot service password, so
	// the VM's console (virsh domdisplay / virt-viewer) is usable for
	// debugging even before the agent connects. SSH stays key-only — the
	// agent pushes the owner's keys once connected.
	osUserEntry := map[string]interface{}{
		"name":        osUser,
		"sudo":        "ALL=(ALL) NOPASSWD:ALL",
		"groups":      []string{"sudo"},
		"shell":       "/bin/bash",
		"lock_passwd": true,
	}
	if user.ServicePassword != "" {
		if hash, err := bcrypt.GenerateFromPassword([]byte(user.ServicePassword), bcrypt.DefaultCost); err == nil {
			osUserEntry["passwd"] = string(hash)
			osUserEntry["lock_passwd"] = false
		}
	}

	ud := userData{
		Hostname:  hostname,
		Timezone:  timezone,
		ManageEtc: true,
		SSHPwAuth: false,
		Users:     []map[string]interface{}{osUserEntry},
		WriteFiles: []map[string]interface{}{
			{
				"path":        "/etc/knot/agent.env",
				"owner":       "root:root",
				"permissions": "0600",
				"content":     strings.Join(env, "\n") + "\n",
			},
			{
				"path":        "/usr/local/sbin/knot-agent-install.sh",
				"owner":       "root:root",
				"permissions": "0755",
				"content":     agentInstallScript,
			},
			{
				"path":        "/usr/local/sbin/knot-agent-run.sh",
				"owner":       "root:root",
				"permissions": "0700",
				"content":     agentRunScript(osUser, env),
			},
			{
				"path":        "/etc/systemd/system/knot-agent.service",
				"owner":       "root:root",
				"permissions": "0644",
				"content":     agentUnit,
			},
		},
		PackageUpdate:  false,
		PackageUpgrade: false,
		RunCmd: []string{
			"systemctl daemon-reload",
			"install -d -o " + osUser + " -g " + osUser + " -m 0750 /home/" + osUser,
			"systemctl disable --now apt-daily.timer apt-daily-upgrade.timer || true",
			"systemctl enable --now knot-agent.service",
		},
	}
	// The guest agent lets operators see a bridged VM's address via
	// 'virsh domifaddr --source agent' — with purely static cloud-init
	// networking the DHCP lease table is always empty. NAT mode is served
	// by the libvirt leases table instead, so it installs nothing and its
	// first boot runs no apt at all.
	if plan != nil {
		ud.Packages = []string{"qemu-guest-agent"}
	}

	userDataYAML, err := yaml.Marshal(ud)
	if err == nil {
		userDataYAML = append([]byte("#cloud-config\n"), userDataYAML...)
	} else {
		userDataYAML = []byte("#cloud-config\nusers:\n  - default\n")
	}

	type nicConfig struct {
		Match       map[string]interface{}   `yaml:"match"`
		Addresses   []string                 `yaml:"addresses"`
		Routes      []map[string]interface{} `yaml:"routes"`
		Nameservers map[string]interface{}   `yaml:"nameservers"`
	}
	type networkConfigV2 struct {
		Version   int                  `yaml:"version"`
		Ethernets map[string]nicConfig `yaml:"ethernets"`
	}

	// Bridged spaces render their static network plan; NAT mode ships no
	// network-config at all — the guest's cloud-init fallback DHCPs from the
	// libvirt network (which also makes the address visible via
	// 'virsh domifaddr --source lease').
	var networkYAML []byte
	if plan != nil {
		dns := make([]string, 0, len(plan.DNS))
		for _, server := range plan.DNS {
			dns = append(dns, server.String())
		}

		nc := networkConfigV2{
			Version: 2,
			Ethernets: map[string]nicConfig{
				"nic0": {
					// Match any kernel interface name: virtio NICs come up as
					// eth0 or a predictable en* name depending on the guest.
					Match:     map[string]interface{}{"name": "e*"},
					Addresses: []string{fmt.Sprintf("%s/%d", plan.IP, plan.PrefixLen)},
					Routes: []map[string]interface{}{
						{"to": "default", "via": plan.Gateway.String()},
					},
					Nameservers: map[string]interface{}{"addresses": dns},
				},
			},
		}
		networkYAML, _ = yaml.Marshal(nc)
	}

	// cloud-init applies network configuration and the per-instance user-data
	// modules exactly once per instance-id. Deriving the id from a hash of
	// the seed's own content means any change to the space's IP, network or
	// agent environment yields a new instance on the next boot and
	// cloud-init re-applies everything — while an unchanged seed keeps the
	// id stable and boots fast. The user-data and network-config are
	// idempotent (same user, same files), so a re-run is safe.
	seedHash := sha256.Sum256(append(append(userDataYAML, networkYAML...), []byte(hostname)...))
	meta := fmt.Sprintf("instance-id: knot-%s-%s\nlocal-hostname: %s\n", space.Id, hex.EncodeToString(seedHash[:])[:12], hostname)

	return &cloudInitData{
		UserData:      string(userDataYAML),
		NetworkConfig: string(networkYAML),
		MetaData:      meta,
	}
}

// agentInstallScript fetches the agent binary from the knot server. It is
// idempotent and tolerates cloud images without unzip (python3 is present
// wherever cloud-init is).
const agentInstallScript = `#!/bin/sh
set -e

# Fetch the agent once per boot: /run is tmpfs, so the marker vanishes on
# reboot and the next start pulls the current build; service restarts within
# a boot reuse it instead of re-downloading on every retry.
if [ -x /usr/local/bin/knot ] && [ -f /run/knot-agent-fetched ]; then
  exit 0
fi

case "$(uname -m)" in
  x86_64)  AGENT_ARCH=amd64 ;;
  aarch64) AGENT_ARCH=arm64 ;;
  *) echo "knot-agent: unsupported architecture $(uname -m)" >&2; exit 1 ;;
esac

. /etc/knot/agent.env
export KNOT_SERVER

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

URL="$KNOT_SERVER/agents/knot_agent_linux_${AGENT_ARCH}.zip"
if command -v curl >/dev/null 2>&1; then
  curl -fsSL "$URL" -o "$TMP/agent.zip"
elif command -v wget >/dev/null 2>&1; then
  wget -q -O "$TMP/agent.zip" "$URL"
else
  echo "knot-agent: no curl or wget available to fetch the agent" >&2
  exit 1
fi

if command -v unzip >/dev/null 2>&1; then
  unzip -o "$TMP/agent.zip" -d "$TMP/out"
else
  python3 -c "import zipfile; zipfile.ZipFile('$TMP/agent.zip').extractall('$TMP/out')"
fi

install -m 0755 "$TMP/out/knot-agent" /usr/local/bin/knot
touch /run/knot-agent-fetched
`

// agentUnit runs the space agent. The unit itself runs as root — no User=,
// no WorkingDirectory, so systemd never performs a setuid/chdir dance that
// can fail on oddly-permissioned home directories — and drops to the space
// owner inside the run script via sudo, the same way an admin would by hand.
const agentUnit = `[Unit]
Description=Knot space agent
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
ExecStartPre=/bin/sh /usr/local/sbin/knot-agent-install.sh
ExecStart=/bin/sh /usr/local/sbin/knot-agent-run.sh
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`

// agentRunScript builds the root-owned launcher that starts the agent as
// the space owner. The environment is embedded as explicit sudo VAR=value
// assignments (quoteEnvEntry already renders shell-safe single-quoted
// tokens) so sudo's env_reset cannot strip it, and -H gives the agent the
// owner's $HOME. The cd is best-effort: a broken home degrades to running
// from / rather than failing the service.
func agentRunScript(osUser string, env []string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\n# Generated by knot — starts the space agent as the space owner.\nset -e\n\nexec sudo -u ")
	b.WriteString(osUser)
	b.WriteString(" -H \\\n")
	for _, entry := range env {
		b.WriteString("  ")
		b.WriteString(entry)
		b.WriteString(" \\\n")
	}
	b.WriteString("  /bin/sh -c 'cd \"$HOME\" 2>/dev/null || true; exec /usr/local/bin/knot agent start'\n")
	return b.String()
}

// isoTools are the ISO 9660 creators tried, in order, when building the
// NoCloud seed image. Any one of them is sufficient.
var isoTools = [][]string{
	{"genisoimage"},
	{"mkisofs"},
	{"xorriso", "-as", "mkisofs"},
}

// writeCloudInitSeed writes the seed files into dir and builds the cidata
// ISO next to them. Returns the ISO path.
func (c *KVMClient) writeCloudInitSeed(ctx context.Context, dir string, data *cloudInitData) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	files := map[string]string{
		"user-data": data.UserData,
		"meta-data": data.MetaData,
	}
	// Absent in NAT mode: without a network-config file the guest falls
	// back to DHCP on the libvirt network.
	if data.NetworkConfig != "" {
		files["network-config"] = data.NetworkConfig
	}
	// 0600: the seed carries the agent registration key, the service
	// password and its hash. libvirt's dynamic ownership chowns the ISO to
	// the qemu user when the domain starts, so the VM can still read it.
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0600); err != nil {
			return "", err
		}
	}

	iso := filepath.Join(filepath.Dir(dir), "seed.iso")
	// Build into a fresh temp file and rename over any existing ISO. The
	// previous seed may still be owned by the qemu user: libvirt's dynamic
	// ownership chowns domain disks at start but does not reliably hand
	// them back when a persistent domain shuts down, and the ISO tools
	// cannot open such a file for writing. Replacing a directory entry
	// only needs write permission on the directory, which knot owns.
	isoTmp := iso + ".new"
	os.Remove(isoTmp)
	for _, tool := range isoTools {
		if _, err := exec.LookPath(tool[0]); err != nil {
			continue
		}
		args := append(append([]string{}, tool[1:]...), "-output", isoTmp, "-volid", "cidata", "-joliet", "-rock", dir)
		if _, err := c.runCommand(ctx, tool[0], args...); err != nil {
			os.Remove(isoTmp)
			return "", err
		}
		if err := os.Chmod(isoTmp, 0600); err != nil {
			os.Remove(isoTmp)
			return "", err
		}
		if err := os.Rename(isoTmp, iso); err != nil {
			os.Remove(isoTmp)
			return "", err
		}
		return iso, nil
	}

	return "", fmt.Errorf("no ISO tool found (install genisoimage, mkisofs or xorriso)")
}

// quoteEnvEntry rewrites a KEY=value entry with a shell-safe, systemd-safe
// single-quoted value: embedded single quotes become '\” (close quote,
// escaped quote, reopen) and line breaks are stripped — an env file line can
// never be allowed to become multiple lines.
func quoteEnvEntry(entry string) string {
	key, value, found := strings.Cut(entry, "=")
	if !found {
		return entry
	}
	value = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, value)
	value = strings.ReplaceAll(value, "'", `'\''`)
	return key + "='" + value + "'"
}
