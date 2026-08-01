package specwizard

import (
	"strings"

	"github.com/paularlott/knot/apiclient"
)

// This file holds the Linux capability catalog the wizard's searchable picker
// renders, plus the normalisation helpers used by both the container YAML and
// Nomad HCL paths.
//
// Canonical form inside UnifiedSpec is upper-case with the CAP_ prefix
// (CAP_NET_ADMIN). That matches the local container spec's validator
// (internal/specvalidate.validateCapability) and gives the UI a single form to
// match against the catalog. Emitters convert to whatever the target runtime
// expects: container YAML keeps the canonical form, Nomad HCL emits the
// documented bare lower-case form (net_admin) unless the original job already
// used CAP_-prefixed names.

// capabilityCatalog is the list offered by the wizard picker. It's the common
// subset rather than the full kernel list — anything missing can still be
// typed into the raw spec, and unknown-but-well-formed names round-trip
// through the wizard untouched.
//
// Common marks the capabilities most often needed by dev spaces; the UI floats
// those to the top of the list.
var capabilityCatalog = []apiclient.CapabilityEntry{
	{Name: "CAP_AUDIT_CONTROL", Description: "Enable and disable kernel auditing and change audit filter rules."},
	{Name: "CAP_AUDIT_WRITE", Description: "Write records to the kernel audit log. Needed by login and su.", Common: true},
	{Name: "CAP_BPF", Description: "Use privileged BPF operations such as loading programs and creating maps."},
	{Name: "CAP_CHOWN", Description: "Change the owner and group of any file.", Common: true},
	{Name: "CAP_DAC_OVERRIDE", Description: "Bypass file read, write and execute permission checks."},
	{Name: "CAP_DAC_READ_SEARCH", Description: "Bypass file read permission checks and directory read and execute checks."},
	{Name: "CAP_FOWNER", Description: "Bypass permission checks on operations that normally require the file owner's UID."},
	{Name: "CAP_FSETID", Description: "Keep the set-user-ID and set-group-ID bits when a file is modified."},
	{Name: "CAP_IPC_LOCK", Description: "Lock memory into RAM with mlock, mlockall and shared memory locking."},
	{Name: "CAP_IPC_OWNER", Description: "Bypass permission checks on System V IPC objects."},
	{Name: "CAP_KILL", Description: "Send signals to any process, bypassing permission checks."},
	{Name: "CAP_LINUX_IMMUTABLE", Description: "Set the immutable and append-only inode flags."},
	{Name: "CAP_MKNOD", Description: "Create special files with mknod, such as device nodes.", Common: true},
	{Name: "CAP_NET_ADMIN", Description: "Perform network administration: interfaces, routing tables, firewall rules, VPNs.", Common: true},
	{Name: "CAP_NET_BIND_SERVICE", Description: "Bind sockets to privileged ports below 1024.", Common: true},
	{Name: "CAP_NET_BROADCAST", Description: "Send socket broadcasts and listen to multicast."},
	{Name: "CAP_NET_RAW", Description: "Use raw and packet sockets. Needed by ping, traceroute and tcpdump.", Common: true},
	{Name: "CAP_PERFMON", Description: "Access performance monitoring and observability interfaces such as perf_events."},
	{Name: "CAP_SETFCAP", Description: "Set file capabilities on executables."},
	{Name: "CAP_SETGID", Description: "Change the group ID of a process and manipulate the supplementary group list."},
	{Name: "CAP_SETPCAP", Description: "Add or drop capabilities from the process capability sets."},
	{Name: "CAP_SETUID", Description: "Change the user ID of a process. Needed by su and sudo.", Common: true},
	{Name: "CAP_SYS_ADMIN", Description: "Wide-ranging system administration: mount, sethostname, namespaces and more.", Common: true},
	{Name: "CAP_SYS_BOOT", Description: "Reboot the system and load a new kernel for later execution."},
	{Name: "CAP_SYS_CHROOT", Description: "Use chroot and change mount namespaces."},
	{Name: "CAP_SYS_MODULE", Description: "Load and unload kernel modules."},
	{Name: "CAP_SYS_NICE", Description: "Raise process priority and set CPU affinity and scheduling policies."},
	{Name: "CAP_SYS_PTRACE", Description: "Trace arbitrary processes with ptrace. Needed by debuggers such as gdb and delve.", Common: true},
	{Name: "CAP_SYS_RAWIO", Description: "Perform raw I/O port operations and access /dev/mem."},
	{Name: "CAP_SYS_RESOURCE", Description: "Override resource limits, disk quotas and reserved space."},
	{Name: "CAP_SYS_TIME", Description: "Set the system clock and the real-time hardware clock.", Common: true},
	{Name: "CAP_SYS_TTY_CONFIG", Description: "Configure TTY devices and use vhangup."},
	{Name: "CAP_SYSLOG", Description: "Read and control the kernel log ring buffer, as used by dmesg."},
}

// Capabilities returns the wizard's capability catalog. The slice is copied so
// callers (including the API handler) can't mutate the package-level catalog.
func Capabilities() []apiclient.CapabilityEntry {
	out := make([]apiclient.CapabilityEntry, len(capabilityCatalog))
	copy(out, capabilityCatalog)
	return out
}

// NormaliseCapability converts a capability name into the canonical
// CAP_UPPER_SNAKE form used inside UnifiedSpec. Accepts the bare form
// (net_admin), mixed case, surrounding whitespace and the already-canonical
// form. Returns "" when the name isn't a well-formed capability so callers can
// drop it rather than writing garbage into a spec.
func NormaliseCapability(name string) string {
	n := strings.ToUpper(strings.TrimSpace(name))
	if n == "" {
		return ""
	}
	// Reject names that would need quoting or can't be a capability at all
	// before the CAP_ prefix is applied.
	body := strings.TrimPrefix(n, "CAP_")
	if body == "" {
		return ""
	}
	for i, r := range body {
		switch {
		case r >= 'A' && r <= 'Z':
		case r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return ""
		}
	}
	return "CAP_" + body
}

// NormaliseCapabilities canonicalises a list of capability names, dropping
// blanks and malformed entries and de-duplicating while preserving the caller's
// order. Returns nil for an empty result so emitters omit the field entirely.
func NormaliseCapabilities(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, c := range in {
		n := NormaliseCapability(c)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// capabilityBareName strips the CAP_ prefix and lower-cases the result, which
// is the form the Nomad docker driver documents (cap_add = ["net_admin"]).
func capabilityBareName(name string) string {
	n := NormaliseCapability(name)
	if n == "" {
		return ""
	}
	return strings.ToLower(strings.TrimPrefix(n, "CAP_"))
}
