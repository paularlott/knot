package specwizard

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/container"
	"gopkg.in/yaml.v3"
)

// HCLParser parses a Nomad HCL job string into the JSON shape returned by
// Nomad's /v1/jobs/parse endpoint. Implementations typically wrap a Nomad
// client; tests can supply a fake. A nil parser causes ParseNomadHCL to
// report the spec as not wizardable.
type HCLParser func(hcl string) (map[string]interface{}, error)

// ParseNomadHCL converts a Nomad HCL job into UnifiedSpec. The HCL must be
// parseable by the supplied parser (typically Nomad's /v1/jobs/parse), and
// must contain exactly one task with driver = "docker" for the result to be
// wizardable. Anything more complex (multi-task, non-docker drivers, multiple
// groups) returns wizardable=false so the UI disables the wizard button.
//
// volumes is the Volume Definition (YAML) text from the template; it is
// parsed into VolumeDefinitions independently of the HCL.
func ParseNomadHCL(job, volumes string, parser HCLParser) (spec *apiclient.UnifiedSpec, wizardable bool, reason string) {
	if strings.TrimSpace(job) == "" {
		// Empty job: the wizard will emit a fresh one.
		storage := parseNomadVolumeDefinitionsToStorage(volumes)
		return &apiclient.UnifiedSpec{
			Storage: storage,
		}, true, ""
	}

	if parser == nil {
		return nil, false, "no HCL parser configured (Nomad not available)"
	}

	// Escape ${ to $${ so Nomad's HCL parser treats ${{ ... }} and ${ ... }
	// as literal strings instead of trying to evaluate them as HCL2 template
	// sequences. Without this, specs containing knot template variables
	// (e.g. ${{ .user.timezone }}, ${{ .space.id }}) fail to parse and the
	// extracted values lose the variable reference. HCL's $${ is the
	// literal-$ escape, so the parsed JSON carries the original ${{ ... }}
	// text back to the wizard verbatim.
	//
	// %{ gets the same treatment: it opens an HCL template *directive*
	// (%{ if ... }), so an unescaped %{ inside a string value is a parse
	// error even though it's meaningless to knot. Escaping is safe to apply
	// to already-escaped input: "$${" becomes "$$${", which HCL renders back
	// to "$${".
	// Comment out standalone Go-template directive lines (e.g. ${{ if .X }},
	// ${{ end }}, ${{ range .X }}) so Nomad's HCL parser doesn't choke on
	// them. These are valid in knot templates — they're evaluated by knot's
	// template engine BEFORE the HCL reaches Nomad — but they're invalid as
	// standalone HCL syntax. Only lines that are ENTIRELY a ${{ ... }}
	// expression are affected; variables inside quoted strings are untouched
	// because those lines don't start with ${{.
	preprocessed := commentTemplateDirectives(job)
	escaped := strings.ReplaceAll(preprocessed, "${", "$${")
	escaped = strings.ReplaceAll(escaped, "%{", "%%{")
	parsed, err := parser(escaped)
	if err != nil {
		return nil, false, fmt.Sprintf("HCL parse failed: %s", err.Error())
	}

	spec, wizardable, reason = extractFromParsedJob(parsed)
	if !wizardable {
		return nil, false, reason
	}
	// Fallback: if the JSON extraction didn't yield auth (some Nomad versions
	// place auth differently or omit it from the parsed output), try
	// extracting from the raw HCL text.
	if spec.Auth == nil {
		spec.Auth = extractAuthFromHCL(job)
	}
	// The job label (job "<name>" { ... }) is extracted from the raw HCL
	// rather than the parsed JSON. Nomad's parser resolves the job's ID/Name
	// fields through its own interpolation, which doesn't reliably preserve
	// knot's ${{ ... }} variable syntax the way the escape trick above does
	// for string values inside blocks.
	spec.Name = extractJobLabel(job)
	spec.Storage = mergeNomadStorage(spec.Storage, volumes)

	// The Nomad parse API doesn't populate the Data field for embedded
	// templates (heredoc content), and the mount key name varies by version.
	// Extract both directly from the raw HCL.
	if len(spec.Templates) > 0 {
		tmplData := extractTemplateDataFromHCL(job)
		mountData := extractTemplateMountsFromHCL(job)
		for i := range spec.Templates {
			if spec.Templates[i].Data == "" {
				spec.Templates[i].Data = tmplData[spec.Templates[i].Destination]
			}
			if mnt, ok := mountData[spec.Templates[i].Destination]; ok {
				spec.Templates[i].MountTarget = mnt.Target
				spec.Templates[i].MountReadonly = mnt.ReadOnly
			}
		}
	}

	return spec, true, ""
}

// extractFromParsedJob walks the JSON job shape and pulls out the wizard-
// editable fields. Returns wizardable=false unless the job has exactly one
// task whose driver is "docker".
func extractFromParsedJob(parsed map[string]interface{}) (*apiclient.UnifiedSpec, bool, string) {
	spec := &apiclient.UnifiedSpec{}

	groups, _ := parsed["TaskGroups"].([]interface{})
	if len(groups) == 0 {
		return nil, false, "job has no task groups"
	}
	if len(groups) > 1 {
		return nil, false, "job has multiple task groups (not editable via wizard)"
	}
	group, _ := groups[0].(map[string]interface{})

	tasks, _ := group["Tasks"].([]interface{})
	if len(tasks) == 0 {
		return nil, false, "task group has no tasks"
	}
	if len(tasks) > 1 {
		return nil, false, "task group has multiple tasks (not editable via wizard)"
	}
	task, _ := tasks[0].(map[string]interface{})

	driver, _ := task["Driver"].(string)
	if driver != "docker" && driver != "podman" {
		return nil, false, fmt.Sprintf("task driver %q is not supported by the wizard (only \"docker\" or \"podman\")", driver)
	}
	spec.Driver = driver

	// Image + config (docker/podman driver: image, command, args, hostname,
	// privileged, auth all live inside the task's Config map).
	configMap, _ := task["Config"].(map[string]interface{})
	if img, ok := configMap["image"].(string); ok {
		spec.Image = img
	}
	if cmd, ok := configMap["command"].([]interface{}); ok {
		spec.Command = interfaceSliceToStrings(cmd)
	}
	if args, ok := configMap["args"].([]interface{}); ok {
		// Nomad merges command+args; expose as Command for the wizard.
		spec.Command = append(spec.Command, interfaceSliceToStrings(args)...)
	}
	if hn, ok := configMap["hostname"].(string); ok {
		spec.Hostname = hn
	}
	if priv, ok := configMap["privileged"].(bool); ok {
		spec.Privileged = priv
	}
	// Linux capabilities. The docker driver takes them as bare lower-case
	// names (cap_add = ["net_admin"]) but also accepts the CAP_-prefixed
	// form; normalise both to the canonical CAP_UPPER form for the wizard.
	spec.CapAdd = NormaliseCapabilities(toStringSlice(configMap["cap_add"]))
	spec.CapDrop = NormaliseCapabilities(toStringSlice(configMap["cap_drop"]))
	if auth, _ := configMap["auth"].(map[string]interface{}); auth != nil {
		u, _ := auth["username"].(string)
		p, _ := auth["password"].(string)
		if u != "" || p != "" {
			spec.Auth = &apiclient.RegistryAuth{Username: u, Password: p}
		}
	}

	// Environment variables
	if env, _ := task["Env"].(map[string]interface{}); env != nil {
		// Build ordered env list: Nomad returns env as a map, which loses the
		// user's insertion order. Sort keys alphabetically for a stable
		// presentation in the wizard; the user can re-order in the UI.
		keys := make([]string, 0, len(env))
		for k := range env {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// Reuse the shared container.ParseEnvStrings helper so the
		// "KEY=value" parsing stays in one place across the codebase.
		raw := make([]string, 0, len(keys))
		for _, k := range keys {
			v, _ := env[k].(string)
			raw = append(raw, k+"="+v)
		}
		for _, ev := range container.ParseEnvStrings(raw) {
			spec.Environment = append(spec.Environment, apiclient.KeyValue{Key: ev.Key, Value: ev.Value})
		}
	}

	// Resources
	if res, _ := task["Resources"].(map[string]interface{}); res != nil {
		if memMB, ok := toFloat(res["MemoryMB"]); ok && memMB > 0 {
			spec.Memory = formatMemory(int64(memMB))
		}
		// Nomad has two CPU fields: `cores` (whole CPU cores, integer) and
		// `cpu` (MHz, integer). They're semantically different and NOT
		// interchangeable. Preserve whichever the user set rather than
		// converting. Default to cpu (MHz) for new specs.
		if cores, ok := toFloat(res["Cores"]); ok && cores > 0 {
			spec.CPUs = strconv.FormatInt(int64(cores), 10)
			spec.CPUType = "cores"
		} else if cpuMHz, ok := toFloat(res["CPU"]); ok && cpuMHz > 0 {
			spec.CPUs = strconv.FormatInt(int64(cpuMHz), 10)
			spec.CPUType = "mhz"
		}
	}

	// Volume mounts (task level). These reference the group-level `volume "name" {}`
	// stanzas. At parse time we only know the mount side (name + destination);
	// the full volume definition comes from the Volume Definition YAML parsed
	// separately. mergeNomadStorage (called by ParseNomadHCL after this returns)
	// unifies the two into StorageEntry rows.
	if mounts, _ := task["VolumeMounts"].([]interface{}); mounts != nil {
		for _, m := range mounts {
			mm, _ := m.(map[string]interface{})
			if mm == nil {
				continue
			}
			volName, _ := mm["Volume"].(string)
			dest, _ := mm["Destination"].(string)
			readonly, _ := mm["ReadOnly"].(bool)
			if volName == "" || dest == "" {
				continue
			}
			spec.Storage = append(spec.Storage, apiclient.StorageEntry{
				Kind:          "bind", // tentative; mergeNomadStorage reclassifies when a definition exists
				HostPath:      volName,
				ContainerPath: dest,
				ReadOnly:      readonly,
			})
		}
	}

	// Template blocks (task level). Each carries a heredoc `data`, a
	// `destination` path, and optional change_mode/change_signal. We also
	// look for `mount {}` blocks inside the docker-driver config that bind
	// the rendered file into the container — matched by source path.
	mountsBySource := map[string]map[string]interface{}{}
	if mounts, _ := configMap["Mounts"].([]interface{}); mounts != nil {
		for _, m := range mounts {
			mm, _ := m.(map[string]interface{})
			if mm == nil {
				continue
			}
			src, _ := mm["Source"].(string)
			if src != "" {
				mountsBySource[src] = mm
			}
		}
	}
	if tmpls, _ := task["Templates"].([]interface{}); tmpls != nil {
		for _, t := range tmpls {
			tm, _ := t.(map[string]interface{})
			if tm == nil {
				continue
			}
			dest, _ := tm["DestPath"].(string)
			if dest == "" {
				continue
			}
			data := getStr(tm, "Data")
			// "restart" is Nomad's default change_mode; only store if the
			// user explicitly set it in the HCL.
			cm := getStr(tm, "ChangeMode")
			if cm == "restart" {
				cm = ""
			}
			nt := apiclient.NomadTemplate{
				Destination:  dest,
				Data:         data,
				ChangeMode:   cm,
				ChangeSignal: getStr(tm, "ChangeSignal"),
			}
			if mnt, ok := mountsBySource[dest]; ok {
				nt.MountTarget, _ = mnt["Target"].(string)
				nt.MountReadonly, _ = mnt["ReadOnly"].(bool)
			}
			spec.Templates = append(spec.Templates, nt)
		}
	}

	// Network ports (group level). Nomad exposes ports under several keys
	// depending on version and whether they're static or dynamic:
	//   Ports          — combined (recent Nomad)
	//   ReservedPorts  — static only (static = N)
	//   DynamicPorts   — dynamic only (Nomad assigns host port)
	// We read all three to be defensive. Container port lives in "To";
	// host port in "Value" (static value or 0 for dynamic). The Label is
	// the port-block name and is what config.ports[] refers to.
	if networks, _ := group["Networks"].([]interface{}); len(networks) > 0 {
		net, _ := networks[0].(map[string]interface{})
		// Preserve the network mode. Without this the builder would rewrite
		// `mode = "host"` (or any non-default mode) to bridge on every apply.
		if mode, ok := net["Mode"].(string); ok {
			spec.Network = mode
		}
		seen := map[string]bool{}
		for _, key := range []string{"Ports", "ReservedPorts", "DynamicPorts"} {
			ports, _ := net[key].([]interface{})
			for _, p := range ports {
				pm, _ := p.(map[string]interface{})
				if pm == nil {
					continue
				}
				label, _ := pm["Label"].(string)
				if label == "" || seen[label] {
					continue
				}
				seen[label] = true
				toPort, _ := toInt(pm["To"])
				hostPort, _ := toInt(pm["Value"])
				// For ReservedPorts (static), Value is the static port.
				// For DynamicPorts, Value is 0 (assigned at runtime).
				// For the combined Ports list, Value is the resolved host
				// port (static value or dynamically assigned).
				if toPort == 0 && hostPort > 0 {
					toPort = hostPort // fallback: to not set, use host
				}
				if toPort == 0 {
					continue
				}
				spec.Ports = append(spec.Ports, apiclient.PortMapping{
					Label:         label,
					HostPort:      hostPort,
					ContainerPort: toPort,
					Protocol:      labelToProtocol(label),
				})
			}
		}
	}

	// Refuse if any group-level volume stanza has a label that differs from its
	// source. The wizard can't handle this: volume_mounts reference the label,
	// but Volume Definitions use the source name, so the mismatch makes
	// round-tripping impossible and would produce broken storage entries.
	if groupVols, _ := group["Volumes"].(map[string]interface{}); groupVols != nil {
		for label, gv := range groupVols {
			vol, _ := gv.(map[string]interface{})
			if vol == nil {
				continue
			}
			source, _ := vol["Source"].(string)
			if source != "" && source != label {
				return nil, false, fmt.Sprintf(
					"volume %q has source %q — label differs from source (not editable via wizard, edit the spec directly)",
					label, source)
			}
		}
	}

	return spec, true, ""
}

// BuildNomadHCL produces HCL for a Nomad job from the unified spec.
//
// If originalHCL is empty, a fresh job skeleton is emitted using the standard
// variable placeholders (${{ .space.name }} etc.) that all Nomad templates in
// the knot catalog use.
//
// If originalHCL is non-empty, the wizard-controlled fields are patched into
// the original text via constrained regex on the first task block. Fields the
// wizard doesn't know about (constraints, meta, services, multicleaf stanzas)
// are preserved byte-for-byte. Comments outside patched ranges survive.
func BuildNomadHCL(spec *apiclient.UnifiedSpec, originalHCL string, originalVolumes string) (job, volumes string, err error) {
	if spec == nil {
		return "", "", fmt.Errorf("nil spec")
	}

	if strings.TrimSpace(originalHCL) == "" {
		job = emitDefaultNomadHCL(spec)
	} else {
		job, err = patchNomadHCL(originalHCL, spec)
		if err != nil {
			return "", "", fmt.Errorf("patch HCL: %w", err)
		}
	}

	volumes = buildNomadStorageDefinitions(spec.Storage, originalVolumes)
	return job, volumes, nil
}

// emitDefaultNomadHCL produces a complete Nomad HCL job skeleton from the
// spec. Output is deterministic so re-emitting an unchanged spec is a no-op.
// defaultKnotEnv returns the environment variables knot injects into every
// new template so the in-container agent can reach the server.
func defaultKnotEnv() []apiclient.KeyValue {
	return []apiclient.KeyValue{
		{Key: "KNOT_SERVER", Value: "${{ .server.url }}"},
		{Key: "KNOT_AGENT_ENDPOINT", Value: "${{ .server.agent_endpoint }}"},
		{Key: "KNOT_SPACEID", Value: "${{ .space.id }}"},
		{Key: "KNOT_USER", Value: "${{ .user.username }}"},
		{Key: "KNOT_SERVICE_PASSWORD", Value: "${{ .user.service_password }}"},
		{Key: "KNOT_LOGLEVEL", Value: "info"},
		{Key: "KNOT_VNC_HTTP_PORT", Value: "5680"},
		{Key: "TZ", Value: "${{ .user.timezone }}"},
	}
}

func emitDefaultNomadHCL(spec *apiclient.UnifiedSpec) string {
	name := spec.Name
	if name == "" {
		name = "${{ .space.name }}-${{ .user.username }}"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "job %s {\n", hclQuoted(name))
	b.WriteString(`  group "app" {
    count = 1

`)
	if volStanzas := emitVolumeStanzasFromStorage(storageVolumes(spec.Storage)); volStanzas != "" {
		b.WriteString(volStanzas)
		b.WriteString("\n")
	}

	if netBlock := emitNetworkBlock(spec.Ports, spec.Network); netBlock != "" {
		b.WriteString(netBlock)
		b.WriteString("\n")
	}

	b.WriteString("    task \"app\" {\n")
	driver := spec.Driver
	if driver == "" {
		driver = "docker"
	}
	fmt.Fprintf(&b, "      driver = %q\n\n", driver)
	b.WriteString("      config {\n")
	fmt.Fprintf(&b, "        image = %s\n", hclQuoted(spec.Image))
	if spec.Hostname != "" {
		fmt.Fprintf(&b, "        hostname = %s\n", hclQuoted(spec.Hostname))
	}
	if spec.Privileged {
		b.WriteString("        privileged = true\n")
	}
	if spec.Auth != nil {
		b.WriteString("        auth {\n")
		fmt.Fprintf(&b, "          username = %s\n", hclQuoted(spec.Auth.Username))
		fmt.Fprintf(&b, "          password = %s\n", hclQuoted(spec.Auth.Password))
		b.WriteString("        }\n")
	}
	if len(spec.Command) > 0 {
		fmt.Fprintf(&b, "        args = %s\n", hclList(spec.Command))
	}
	// Fresh specs use the bare lower-case capability form documented by the
	// Nomad docker driver.
	if caps := capabilityHCLList(spec.CapAdd, false); caps != "" {
		fmt.Fprintf(&b, "        cap_add = %s\n", caps)
	}
	if caps := capabilityHCLList(spec.CapDrop, false); caps != "" {
		fmt.Fprintf(&b, "        cap_drop = %s\n", caps)
	}
	if line := emitConfigPortsLine(spec.Ports); line != "" {
		// emitConfigPortsLine returns 2-space indented; config block uses 8.
		b.WriteString("        " + strings.TrimPrefix(line, "  "))
	}
	b.WriteString("      }\n\n")

	// Always emit the env block in a fresh template — the knot agent needs
	// these to connect back to the server. User-added vars are appended after.
	b.WriteString("      env {\n")
	for _, kv := range defaultKnotEnv() {
		fmt.Fprintf(&b, "        %s = %s\n", hclKey(kv.Key), hclQuoted(kv.Value))
	}
	for _, kv := range spec.Environment {
		fmt.Fprintf(&b, "        %s = %s\n", hclKey(kv.Key), hclQuoted(kv.Value))
	}
	b.WriteString("      }\n\n")

	hasResources := spec.Memory != "" || spec.CPUs != ""
	if hasResources {
		b.WriteString("      resources {\n")
		if spec.Memory != "" {
			memMB, _ := memoryToMB(spec.Memory)
			fmt.Fprintf(&b, "        memory = %d\n", memMB)
		}
		if spec.CPUs != "" {
			cpuVal, _ := strconv.ParseInt(spec.CPUs, 10, 64)
			if spec.CPUType == "cores" {
				fmt.Fprintf(&b, "        cores = %d\n", cpuVal)
			} else {
				fmt.Fprintf(&b, "        cpu = %d\n", cpuVal)
			}
		}
		b.WriteString("      }\n\n")
	}

	if vmBlocks := emitVolumeMountBlocksFromStorage(spec.Storage); vmBlocks != "" {
		b.WriteString(vmBlocks)
		b.WriteString("\n")
	}

	if svc := emitServiceBlocks(spec.Ports); svc != "" {
		b.WriteString(svc)
		b.WriteString("\n")
	}

	if tmpl := emitTemplateBlocks(spec.Templates); tmpl != "" {
		b.WriteString(tmpl)
		b.WriteString("\n")
	}

	b.WriteString("    }\n  }\n}\n")
	return b.String()
}

// patchNomadHCL applies wizard-controlled field changes to the original HCL
// text without re-emitting fields it doesn't know about.
//
// Strategy:
//  1. Locate the first task block (regex on `task "..." {` ... matching `}`).
//  2. Within that block, patch:
//     - image (in config {})
//     - args/command (in config {})
//     - env {} block (full block replace)
//     - resources {} block (full block replace)
//     - volume_mount {} blocks (replace as a group)
//  3. Within the first group block, patch:
//     - volume {} stanzas (replace as a group)
//     - network {} block (full block replace)
//
// Patches that can't find their target are inserted at the end of the
// relevant block. Patches that find an empty result in the spec remove the
// target block.
//
// Limitations (documented): the regex-based localisation assumes standard HCL
// formatting. Pathological inputs (e.g. nested string literals containing
// `task "..."`) may confuse the localiser; the parse step's Nomad-backed
// validator catches most of these because such inputs fail to parse as valid
// Nomad jobs in the first place.
func patchNomadHCL(hcl string, spec *apiclient.UnifiedSpec) (string, error) {
	out := hcl

	taskRange, ok := findFirstTaskBlock(out)
	if !ok {
		// No task block to patch. Emitting a default skeleton here would throw
		// away everything the user wrote, so refuse instead — the wizard's
		// parse step already rejects task-less jobs (wizardable=false), so
		// reaching this point means the HCL changed underneath the wizard.
		return "", fmt.Errorf("no task block found in the job — edit the spec directly")
	}
	taskBody := out[taskRange.start:taskRange.end]

	// Patch config { image = ...; args = ... } inside the task body.
	taskBody = patchTaskConfig(taskBody, spec)

	// Replace env {} block wholesale if the spec carries env.
	if len(spec.Environment) > 0 || hasBlock(taskBody, "env") {
		taskBody = replaceOrRemoveBlock(taskBody, "env", emitEnvBlock(spec.Environment))
	}

	// Replace resources {} block.
	if spec.Memory != "" || spec.CPUs != "" || hasBlock(taskBody, "resources") {
		taskBody = replaceOrRemoveBlock(taskBody, "resources", emitResourcesBlock(spec.Memory, spec.CPUs, spec.CPUType))
	}

	// Replace volume_mount {} blocks as a group.
	taskBody = replaceRepeatedBlocks(taskBody, "volume_mount", emitVolumeMountBlocksFromStorage(spec.Storage))

	// Replace service {} blocks as a group — one per port so Consul can
	// discover the space's exposed services.
	taskBody = replaceRepeatedBlocks(taskBody, "service", emitServiceBlocks(spec.Ports))

	// Replace template {} blocks as a group (Nomad template stanzas).
	taskBody = replaceRepeatedBlocks(taskBody, "template", emitTemplateBlocks(spec.Templates))

	out = out[:taskRange.start] + taskBody + out[taskRange.end:]

	// Patch the job label (job "<name>" { ... }). Empty spec.Name leaves the
	// existing label untouched — there's no "clear the job name" concept.
	out = patchJobLabel(out, spec.Name)

	// Now patch the group block (volumes + network).
	groupRange, ok := findFirstGroupBlock(out)
	if ok {
		groupBody := out[groupRange.start:groupRange.end]
		vols := storageVolumes(spec.Storage)
		if len(vols) > 0 || hasRepeatedBlocks(groupBody, "volume") {
			groupBody = replaceRepeatedBlocks(groupBody, "volume", emitVolumeStanzasFromStorage(vols))
		}
		if len(spec.Ports) > 0 || spec.Network != "" || hasBlock(groupBody, "network") {
			groupBody = replaceOrRemoveBlock(groupBody, "network", emitNetworkBlock(spec.Ports, spec.Network))
		}
		out = out[:groupRange.start] + groupBody + out[groupRange.end:]
	}

	return out, nil
}

// --- helpers ---

type byteRange struct{ start, end int }

// findFirstTaskBlock returns the byte range of the *body* of the first
// `task "..." { ... }` block in the HCL (i.e. the range between the opening
// brace and its matching close, exclusive of both braces).
func findFirstTaskBlock(hcl string) (byteRange, bool) {
	re := regexp.MustCompile(`(?m)^\s*task\s+"[^"]*"\s*\{`)
	loc := re.FindStringIndex(hcl)
	if loc == nil {
		return byteRange{}, false
	}
	bodyStart := loc[1] // just after the `{`
	bodyEnd, ok := matchBrace(hcl, bodyStart-1)
	if !ok {
		return byteRange{}, false
	}
	return byteRange{start: bodyStart, end: bodyEnd}, true
}

func findFirstGroupBlock(hcl string) (byteRange, bool) {
	re := regexp.MustCompile(`(?m)^\s*group\s+"[^"]*"\s*\{`)
	loc := re.FindStringIndex(hcl)
	if loc == nil {
		return byteRange{}, false
	}
	bodyStart := loc[1]
	bodyEnd, ok := matchBrace(hcl, bodyStart-1)
	if !ok {
		return byteRange{}, false
	}
	return byteRange{start: bodyStart, end: bodyEnd}, true
}

// matchBrace, given the index of an opening `{` in hcl, returns the index of
// its matching closing `}` (the index of the `}` itself). Quoted strings,
// comments and heredocs are skipped so braces inside them don't affect the
// depth count. Returns ok=false on unbalanced input.
func matchBrace(hcl string, openIdx int) (int, bool) {
	if openIdx < 0 || openIdx >= len(hcl) || hcl[openIdx] != '{' {
		return 0, false
	}
	depth := 0
	i := openIdx
	for i < len(hcl) {
		c := hcl[i]
		switch c {
		case '"':
			// Skip over the string, handling escapes.
			i++
			for i < len(hcl) {
				if hcl[i] == '\\' && i+1 < len(hcl) {
					i += 2
					continue
				}
				if hcl[i] == '"' {
					break
				}
				i++
			}
		case '<':
			// Heredoc (<<TAG or <<-TAG): its body is arbitrary text, commonly
			// a Nomad template full of {{ }} pairs or shell braces, so it must
			// not contribute to brace depth.
			if end, ok := skipHeredoc(hcl, i); ok {
				i = end
				continue
			}
		case '#':
			// Line comment — skip to end of line.
			for i < len(hcl) && hcl[i] != '\n' {
				i++
			}
		case '/':
			if i+1 < len(hcl) && hcl[i+1] == '/' {
				for i < len(hcl) && hcl[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(hcl) && hcl[i+1] == '*' {
				i += 2
				for i+1 < len(hcl) && !(hcl[i] == '*' && hcl[i+1] == '/') {
					i++
				}
				i += 2
				continue
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, true
			}
		}
		i++
	}
	return 0, false
}

// skipHeredoc detects a heredoc opener at index i (`<<TAG` or `<<-TAG`) and
// returns the index just past its terminating line. ok is false when i isn't a
// heredoc opener, in which case the caller continues scanning normally. An
// unterminated heredoc consumes the rest of the input, which makes the enclosing
// matchBrace call fail rather than silently mis-match a brace.
func skipHeredoc(hcl string, i int) (int, bool) {
	if i+2 >= len(hcl) || hcl[i] != '<' || hcl[i+1] != '<' {
		return 0, false
	}
	j := i + 2
	if j < len(hcl) && hcl[j] == '-' {
		j++
	}
	tagStart := j
	for j < len(hcl) && (isIdentByte(hcl[j])) {
		j++
	}
	tag := hcl[tagStart:j]
	if tag == "" {
		return 0, false
	}
	// Skip to the end of the opening line.
	for j < len(hcl) && hcl[j] != '\n' {
		j++
	}
	// Scan lines until one whose trimmed content is exactly the tag.
	for j < len(hcl) {
		j++ // step past the newline
		lineStart := j
		for j < len(hcl) && hcl[j] != '\n' {
			j++
		}
		if strings.TrimSpace(hcl[lineStart:j]) == tag {
			return j, true
		}
	}
	return len(hcl), true
}

func isIdentByte(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

// hasBlock reports whether a block with the given name exists in the body.
func hasBlock(body, name string) bool {
	re := regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(name) + `\s*\{`)
	return re.MatchString(body)
}

// hasRepeatedBlocks reports whether the body contains one or more blocks with
// the given name and an optional quoted label (e.g. `volume "name" {`).
func hasRepeatedBlocks(body, name string) bool {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `\b[^{\n"]*(?:"[^"]*"[^{\n"]*)*\{`)
	return re.MatchString(body)
}

// replaceOrRemoveBlock finds the first block named `name` (e.g. "env") in
// body and replaces its entire `name { ... }` form with replacement. If
// replacement is empty, the block is removed (along with a trailing blank
// line if present). If the block doesn't exist and replacement is non-empty,
// replacement is appended at the end of body. Trailing whitespace is trimmed
// before appending so repeated patch+repatch cycles don't drift.
func replaceOrRemoveBlock(body, name, replacement string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `[ \t]*\{`)
	loc := re.FindStringIndex(body)
	if loc == nil {
		if strings.TrimSpace(replacement) == "" {
			return body
		}
		// Append at the end of the body. Trailing whitespace is normalised so
		// the appended block's position is deterministic across repeated
		// patches (idempotency) while the parent's closing brace keeps its
		// indentation.
		return appendBlockToBody(body, replacement)
	}
	// Find the matching closing brace starting at loc[1]-1 (the `{`).
	closeIdx, ok := matchBrace(body, loc[1]-1)
	if !ok {
		return body
	}
	// Include the trailing newline so we don't leave blank lines.
	end := closeIdx + 1
	if end < len(body) && body[end] == '\n' {
		end++
	}
	if strings.TrimSpace(replacement) == "" {
		return body[:loc[0]] + body[end:]
	}
	return body[:loc[0]] + replacement + body[end:]
}

// replaceRepeatedBlocks handles block names that can appear multiple times
// consecutively (volume, volume_mount, service). All existing instances are
// removed and replacement (if any) is inserted at the position of the first
// one.
//
// The regex uses `\b` after the name so "volume" doesn't match "volume_mount".
// The `[^{\n"]*(?:"[^"]*"[^{\n"]*)*` segment handles optional quoted labels
// (e.g. `volume "name" {`) while correctly skipping `${{ }}` variable braces
// inside the quoted string — a plain `[^{]*` would stop at the first `{` in
// `${{` and corrupt the HCL.
func replaceRepeatedBlocks(body, name, replacement string) string {
	re := regexp.MustCompile(`(?m)^[ \t]*` + regexp.QuoteMeta(name) + `\b[^{\n"]*(?:"[^"]*"[^{\n"]*)*\{`)
	locs := re.FindAllStringIndex(body, -1)
	if len(locs) == 0 {
		if replacement == "" {
			return body
		}
		return appendBlockToBody(body, replacement)
	}
	first := locs[0][0]
	// Walk from the last match to the first, removing each block + trailing newline.
	for i := len(locs) - 1; i >= 0; i-- {
		open := locs[i][1] - 1 // index of `{`
		closeIdx, ok := matchBrace(body, open)
		if !ok {
			continue
		}
		end := closeIdx + 1
		if end < len(body) && body[end] == '\n' {
			end++
		}
		body = body[:locs[i][0]] + body[end:]
	}
	if replacement == "" {
		return body
	}
	return body[:first] + replacement + body[first:]
}

// appendBlockToBody appends a block to the end of a parent block's body while
// keeping the whitespace that precedes the parent's closing brace, so the
// parent's `}` keeps its original indentation. replacement is expected to end
// with a newline.
func appendBlockToBody(body, replacement string) string {
	trimmed := strings.TrimRight(body, " \t\n")
	trailing := body[len(trimmed):]
	closingIndent := ""
	if i := strings.LastIndexByte(trailing, '\n'); i >= 0 {
		closingIndent = trailing[i+1:]
	}
	if !strings.HasSuffix(replacement, "\n") {
		replacement += "\n"
	}
	return trimmed + "\n" + replacement + closingIndent
}

// patchTaskConfig patches the docker-driver config { ... } block inside the
// task body. It handles image, args, hostname, privileged and auth — all of
// which live in the same Config map on the Nomad side.
//
// Field regexes use the `((?:^|[;{])[ \t]*)` prefix so they match whether
// the config block is formatted across multiple lines:
//
//	config {
//	  image = "..."
//	}
//
// or compactly on one line:
//
//	config { image = "..." }
//
// The `[ \t]*` after the anchor consumes leading indentation in both cases
// (essential for multiline-formatted blocks where fields are indented). The
// alternation also avoids matching field names inside comments (# image = ...)
// since the comment's leading `#` is neither line-start nor `;`/`{`.
func patchTaskConfig(body string, spec *apiclient.UnifiedSpec) string {
	// Find the config block.
	re := regexp.MustCompile(`(?m)^[ \t]*config[ \t]*\{`)
	loc := re.FindStringIndex(body)
	if loc == nil {
		// No config block in the task. If the spec carries config-level fields,
		// emit a fresh config block and insert it after the driver line (or at
		// the start of the task body as a fallback).
		if !hasConfigLevelFields(spec) {
			return body
		}
		configBlock := emitConfigBlock(spec)
		return insertConfigBlock(body, configBlock)
	}
	blockStart := loc[0]
	closeIdx, ok := matchBrace(body, loc[1]-1)
	if !ok {
		return body
	}
	blockEnd := closeIdx + 1
	configBlock := body[blockStart:blockEnd]

	// Patch image.
	if spec.Image != "" {
		imgRe := regexp.MustCompile(`(?m)((?:^|[;{])[ \t]*)(image[ \t]*=[ \t]*)"[^"]*"`)
		if imgRe.MatchString(configBlock) {
			configBlock = imgRe.ReplaceAllString(configBlock, fmt.Sprintf(`${1}${2}%s`, hclQuoted(spec.Image)))
		} else {
			configBlock = insertConfigField(configBlock, "image = "+hclQuoted(spec.Image))
		}
	}

	// Patch args (Nomad uses args, not command, for docker driver).
	argsLiteral := ""
	if len(spec.Command) > 0 {
		argsLiteral = hclList(spec.Command)
	}
	configBlock = patchConfigListField(configBlock, "args", argsLiteral)

	// Patch cap_add / cap_drop. The list style follows whatever the original
	// block used (bare `net_admin` vs `CAP_NET_ADMIN`) so applying the wizard
	// doesn't reformat capabilities the user didn't touch.
	usePrefix := hclCapsUseCapPrefix(configBlock)
	configBlock = patchConfigListField(configBlock, "cap_add", capabilityHCLList(spec.CapAdd, usePrefix))
	configBlock = patchConfigListField(configBlock, "cap_drop", capabilityHCLList(spec.CapDrop, usePrefix))

	// Patch hostname (config-level for docker driver).
	hnRe := regexp.MustCompile(`(?m)((?:^|[;{])[ \t]*)(hostname[ \t]*=[ \t]*)"[^"]*"`)
	if spec.Hostname != "" {
		if hnRe.MatchString(configBlock) {
			configBlock = hnRe.ReplaceAllString(configBlock, fmt.Sprintf(`${1}${2}%s`, hclQuoted(spec.Hostname)))
		} else {
			configBlock = insertConfigField(configBlock, "hostname = "+hclQuoted(spec.Hostname))
		}
	} else if hnRe.MatchString(configBlock) {
		// Remove the whole line (including its newline) rather than just the
		// value, and keep a leading `{` / `;` so a compact one-line config
		// block isn't broken apart.
		hnLineRe := regexp.MustCompile(`(?m)((?:^|[;{])[ \t]*)hostname[ \t]*=[ \t]*"[^"]*"[ \t]*\n?`)
		configBlock = removeMatchesKeepingPrefix(configBlock, hnLineRe)
	}

	// Patch privileged (bool). Allow compact form `config { privileged = true }`.
	// An existing line is always updated (so it can be switched back to false),
	// but a missing line is only added when privileged is on — writing
	// `privileged = false` into every spec would be noise.
	privRe := regexp.MustCompile(`(?m)((?:^|[;{])[ \t]*)privileged[ \t]*=[ \t]*(?:true|false)`)
	if privRe.MatchString(configBlock) {
		configBlock = privRe.ReplaceAllStringFunc(configBlock, func(match string) string {
			subm := privRe.FindStringSubmatch(match)
			if len(subm) < 2 {
				return match
			}
			return subm[1] + "privileged = " + boolStr(spec.Privileged)
		})
	} else if spec.Privileged {
		configBlock = insertConfigField(configBlock, "privileged = true")
	}

	// Patch auth block (inline form: auth { username = "..." password = "..." }).
	// Only replace when spec.Auth is non-nil. When spec.Auth is nil (wizard
	// didn't extract auth, or the user didn't set one), the existing auth
	// block is PRESERVED — removing it would silently drop registry credentials.
	authRe := regexp.MustCompile(`(?ms)^[ \t]*auth[ \t]*\{.*?\n[ \t]*\}`)
	if spec.Auth != nil {
		// Detect the indentation of sibling fields so the auth block matches.
		fieldIndent := detectFieldIndent(configBlock)
		innerIndent := fieldIndent + "  "
		authLiteral := fieldIndent + "auth {\n" +
			innerIndent + "username = " + hclQuoted(spec.Auth.Username) + "\n" +
			innerIndent + "password = " + hclQuoted(spec.Auth.Password) + "\n" +
			fieldIndent + "}\n"
		if authRe.MatchString(configBlock) {
			configBlock = authRe.ReplaceAllString(configBlock, authLiteral)
		} else {
			configBlock = insertAfterBrace(configBlock, authLiteral)
		}
	}

	// Patch ports = [...] (references to network port labels). Keep this line
	// in sync with spec.Ports so the config never references labels that no
	// longer exist in the group's network block.
	configBlock = patchConfigListField(configBlock, "ports", configPortsLiteral(spec.Ports))

	// Replace template-related mount blocks or volumes entries inside config.
	// Docker uses `mount {}` blocks, podman uses `volumes = [...]`.
	if spec.Driver == "podman" {
		configBlock = replaceTemplateVolumes(configBlock, spec.Templates)
	} else {
		configBlock = replaceTemplateMounts(configBlock, spec.Templates)
	}

	return body[:blockStart] + configBlock + body[blockEnd:]
}

// patchConfigListField replaces, inserts or removes a list-valued field
// (`name = [ ... ]`) inside a docker-driver config block.
//
// literal is the bracketed HCL list to write (e.g. `["a", "b"]`); an empty
// literal removes the field. The field regex tolerates both the multi-line and
// compact `config { name = [...] }` layouts, and removal keeps a leading `{`
// or `;` so the compact form isn't broken apart.
func patchConfigListField(configBlock, name, literal string) string {
	re := regexp.MustCompile(`(?m)((?:^|[;{])[ \t]*)` + regexp.QuoteMeta(name) + `[ \t]*=[ \t]*\[[^\]]*\]\n?`)

	if literal == "" {
		if !re.MatchString(configBlock) {
			return configBlock
		}
		return removeMatchesKeepingPrefix(configBlock, re)
	}

	if re.MatchString(configBlock) {
		return re.ReplaceAllStringFunc(configBlock, func(match string) string {
			subm := re.FindStringSubmatch(match)
			if len(subm) < 2 {
				return match
			}
			out := subm[1] + name + " = " + literal
			if strings.HasSuffix(match, "\n") {
				out += "\n"
			}
			return out
		})
	}
	return insertConfigField(configBlock, name+" = "+literal)
}

// removeMatchesKeepingPrefix deletes every match of re, except for a leading
// `{` or `;` captured in group 1. Field regexes in this file anchor on either
// line start or those separators to support compact single-line blocks; dropping
// the separator along with the field would corrupt the HCL.
func removeMatchesKeepingPrefix(block string, re *regexp.Regexp) string {
	return re.ReplaceAllStringFunc(block, func(match string) string {
		subm := re.FindStringSubmatch(match)
		if len(subm) >= 2 && (strings.HasPrefix(subm[1], "{") || strings.HasPrefix(subm[1], ";")) {
			return subm[1]
		}
		return ""
	})
}

// insertConfigField inserts a single `key = value` assignment at the top of a
// config block, indented to match the block's existing fields.
func insertConfigField(configBlock, assignment string) string {
	return insertAfterBrace(configBlock, detectFieldIndent(configBlock)+assignment+"\n")
}

// hclCapsUseCapPrefix reports whether the block's existing cap_add / cap_drop
// entries are written in the CAP_-prefixed form. Used to keep the wizard's
// output in the style the author already chose.
func hclCapsUseCapPrefix(configBlock string) bool {
	re := regexp.MustCompile(`(?mi)((?:^|[;{])[ \t]*)cap_(?:add|drop)[ \t]*=[ \t]*\[[^\]]*CAP_`)
	return re.MatchString(configBlock)
}

// capabilityHCLList renders a capability list as an HCL list literal. With
// useCapPrefix the canonical CAP_UPPER form is emitted, otherwise the bare
// lower-case form the Nomad docker driver documents (["net_admin"]). Returns
// an empty string when there's nothing to emit so callers remove the field.
func capabilityHCLList(caps []string, useCapPrefix bool) string {
	normalised := NormaliseCapabilities(caps)
	if len(normalised) == 0 {
		return ""
	}
	out := make([]string, 0, len(normalised))
	for _, c := range normalised {
		if useCapPrefix {
			out = append(out, c)
		} else {
			out = append(out, capabilityBareName(c))
		}
	}
	return hclList(out)
}

func insertAfterBrace(block, snippet string) string {
	// block looks like `config {\n...}`. Find first `{` and insert just after it.
	idx := strings.IndexByte(block, '{')
	if idx < 0 {
		return block
	}
	rest := block[idx+1:]
	if !strings.HasPrefix(rest, "\n") {
		rest = "\n" + rest
	}
	return block[:idx+1] + rest[:1] + snippet + rest[1:]
}

// hasConfigLevelFields reports whether the spec carries any field that lives
// inside a docker-driver config {} block. Used to decide whether to emit a
// fresh config block when the task doesn't have one.
func hasConfigLevelFields(spec *apiclient.UnifiedSpec) bool {
	if spec == nil {
		return false
	}
	return spec.Image != "" || spec.Hostname != "" || spec.Privileged ||
		spec.Auth != nil || len(spec.Command) > 0 || len(spec.Ports) > 0 ||
		len(spec.CapAdd) > 0 || len(spec.CapDrop) > 0
}

// emitConfigBlock builds a complete `config { ... }` block from the spec's
// config-level fields. Used when the original HCL has no config block.
func emitConfigBlock(spec *apiclient.UnifiedSpec) string {
	var b strings.Builder
	b.WriteString("      config {\n")
	if spec.Image != "" {
		fmt.Fprintf(&b, "        image = %s\n", hclQuoted(spec.Image))
	}
	if spec.Hostname != "" {
		fmt.Fprintf(&b, "        hostname = %s\n", hclQuoted(spec.Hostname))
	}
	if spec.Privileged {
		b.WriteString("        privileged = true\n")
	}
	if spec.Auth != nil {
		b.WriteString("        auth {\n")
		fmt.Fprintf(&b, "          username = %s\n", hclQuoted(spec.Auth.Username))
		fmt.Fprintf(&b, "          password = %s\n", hclQuoted(spec.Auth.Password))
		b.WriteString("        }\n")
	}
	if len(spec.Command) > 0 {
		fmt.Fprintf(&b, "        args = %s\n", hclList(spec.Command))
	}
	if caps := capabilityHCLList(spec.CapAdd, false); caps != "" {
		fmt.Fprintf(&b, "        cap_add = %s\n", caps)
	}
	if caps := capabilityHCLList(spec.CapDrop, false); caps != "" {
		fmt.Fprintf(&b, "        cap_drop = %s\n", caps)
	}
	if line := emitConfigPortsLine(spec.Ports); line != "" {
		b.WriteString("        " + strings.TrimPrefix(line, "  "))
	}
	b.WriteString("      }\n")
	return b.String()
}

// insertConfigBlock inserts a config block into a task body that doesn't have
// one. It places the block after the `driver = "..."` line if present, else
// at the start of the body.
func insertConfigBlock(body, configBlock string) string {
	driverRe := regexp.MustCompile(`(?m)^[ \t]*driver[ \t]*=[ \t]*"[^"]*"\n`)
	loc := driverRe.FindStringIndex(body)
	if loc != nil {
		return body[:loc[1]] + configBlock + body[loc[1]:]
	}
	// No driver line — insert at the very start.
	return configBlock + body
}

// jobLabelRe matches the job block header: job "<name>" {. Anchored to the
// start of a line (allowing leading whitespace) so it doesn't match a string
// literal elsewhere in the file that happens to contain the word "job".
// commentTemplateDirectives replaces standalone Go-template directive lines
// (e.g. `${{ if .X }}`, `${{ end }}`, `${{ range .X }}`) with HCL comments so
// Nomad's parser doesn't fail on them. Only lines whose ENTIRE trimmed
// content is a ${{ ... }} expression are affected — variables inside quoted
// strings (like `image = "${{ .X }}"`) are on lines that don't start with
// ${{ and are left untouched. Lines inside heredocs whose content happens to
// be a standalone ${{ ... }} are an acceptable casualty (extremely rare in
// practice — heredoc bodies are data, not template logic).
var templateDirectiveRe = regexp.MustCompile(`(?m)^[ \t]*\$\{\{.*\}\}[ \t]*$`)

func commentTemplateDirectives(hcl string) string {
	return templateDirectiveRe.ReplaceAllStringFunc(hcl, func(line string) string {
		trimmed := strings.TrimLeft(line, " \t")
		indent := line[:len(line)-len(trimmed)]
		return indent + "# " + trimmed
	})
}

var jobLabelRe = regexp.MustCompile(`(?m)^([ \t]*job[ \t]+)"([^"]*)"([ \t]*\{)`)

// extractJobLabel returns the job name from `job "<name>" { ... }` in the raw
// HCL text, or "" if no job block is found.
func extractJobLabel(hcl string) string {
	m := jobLabelRe.FindStringSubmatch(hcl)
	if m == nil {
		return ""
	}
	return m[2]
}

// patchJobLabel replaces the name in `job "<name>" { ... }` with name. If the
// HCL has no job block, it's returned unchanged — patchNomadHCL already
// requires a task block to proceed, so a missing job block means the input
// isn't a job at all and BuildNomadHCL will have already errored by the time
// this would matter.
func patchJobLabel(hcl, name string) string {
	if name == "" || !jobLabelRe.MatchString(hcl) {
		return hcl
	}
	// name itself becomes the replacement text, so any literal "$" in it
	// (knot's ${{ ... }} variables are full of them) must be doubled to
	// escape ReplaceAllString's own $N group syntax — regexp.QuoteMeta
	// escapes regex metacharacters, not replacement-string ones, and doesn't
	// help here.
	escaped := strings.ReplaceAll(name, "$", "$$")
	return jobLabelRe.ReplaceAllString(hcl, `$1"`+escaped+`"$3`)
}

// extractTemplateDataFromHCL scans the raw HCL for `template {}` blocks and
// extracts the heredoc data for each, keyed by destination. The Nomad parse
// API doesn't populate the Data field for embedded templates, so we read it
// directly from the source text.
func extractTemplateDataFromHCL(hcl string) map[string]string {
	result := map[string]string{}
	re := regexp.MustCompile(`(?m)^[ \t]*template\b[^{\n"]*(?:"[^"]*"[^{\n"]*)*\{`)
	locs := re.FindAllStringIndex(hcl, -1)
	for _, loc := range locs {
		openIdx := loc[1] - 1
		closeIdx, ok := matchBrace(hcl, openIdx)
		if !ok {
			continue
		}
		block := hcl[loc[0] : closeIdx+1]

		// Extract destination.
		destRe := regexp.MustCompile(`destination\s*=\s*"([^"]*)"`)
		destMatch := destRe.FindStringSubmatch(block)
		if len(destMatch) < 2 {
			continue
		}
		dest := destMatch[1]

		// Extract heredoc data: find `data = <<TAG` or `data = <<-TAG`.
		dataRe := regexp.MustCompile(`data\s*=\s*<<-?([A-Za-z_][A-Za-z0-9_]*)`)
		dataMatch := dataRe.FindStringSubmatch(block)
		if len(dataMatch) < 2 {
			continue
		}
		tag := dataMatch[1]

		// Find content between the opening line and the closing tag line.
		dataAssignIdx := strings.Index(block, dataMatch[0])
		if dataAssignIdx < 0 {
			continue
		}
		// Skip past the opening line (everything up to and including the newline after <<TAG).
		rest := block[dataAssignIdx:]
		newlineIdx := strings.Index(rest, "\n")
		if newlineIdx < 0 {
			continue
		}
		content := rest[newlineIdx+1:]

		// Find the closing tag line and trim everything from it onward.
		tagBytes := []byte(tag)
		lines := strings.Split(content, "\n")
		var contentLines []string
		for _, line := range lines {
			if strings.TrimSpace(line) == string(tagBytes) {
				break
			}
			contentLines = append(contentLines, line)
		}
		result[dest] = strings.Join(contentLines, "\n")
	}
	return result
}

// templateMountInfo holds the container mount details for a template file.
type templateMountInfo struct {
	Target   string
	ReadOnly bool
}

// extractTemplateMountsFromHCL scans the raw HCL for `mount {}` blocks inside
// the first task's `config {}` block whose `source` starts with "local/"
// (i.e. they reference template-rendered files). Returns a map keyed by
// source path.
func extractTemplateMountsFromHCL(hcl string) map[string]templateMountInfo {
	result := map[string]templateMountInfo{}

	taskRange, ok := findFirstTaskBlock(hcl)
	if !ok {
		return result
	}
	taskBody := hcl[taskRange.start:taskRange.end]

	// Find the config block within the task.
	configRe := regexp.MustCompile(`(?m)^[ \t]*config[ \t]*\{`)
	configLoc := configRe.FindStringIndex(taskBody)
	if configLoc == nil {
		return result
	}
	configEnd, ok := matchBrace(taskBody, configLoc[1]-1)
	if !ok {
		return result
	}
	configBlock := taskBody[configLoc[0] : configEnd+1]

	// Find all mount blocks within config (docker driver).
	mountRe := regexp.MustCompile(`(?m)^[ \t]*mount\b[^{\n"]*(?:"[^"]*"[^{\n"]*)*\{`)
	mountLocs := mountRe.FindAllStringIndex(configBlock, -1)
	for _, loc := range mountLocs {
		openIdx := loc[1] - 1
		closeIdx, ok := matchBrace(configBlock, openIdx)
		if !ok {
			continue
		}
		mountBlock := configBlock[loc[0] : closeIdx+1]

		source := extractHCLStringField(mountBlock, "source")
		if !strings.HasPrefix(source, "local/") {
			continue
		}
		result[source] = templateMountInfo{
			Target:   extractHCLStringField(mountBlock, "target"),
			ReadOnly: extractHCLBoolField(mountBlock, "readonly"),
		}
	}

	// Also check volumes = [...] entries (podman driver).
	// Format: "host_path:container_path[:options]" where options can be "ro".
	volRe := regexp.MustCompile(`(?ms)^\s*volumes\s*=\s*\[(.*?)\]`)
	volMatch := volRe.FindStringSubmatch(configBlock)
	if len(volMatch) >= 2 {
		entryRe := regexp.MustCompile(`"([^"]*)"`)
		for _, m := range entryRe.FindAllStringSubmatch(volMatch[1], -1) {
			parts := strings.SplitN(m[1], ":", 3)
			if len(parts) < 2 {
				continue
			}
			source := parts[0]
			if !strings.HasPrefix(source, "local/") {
				continue
			}
			target := parts[1]
			readOnly := len(parts) >= 3 && strings.Contains(parts[2], "ro")
			result[source] = templateMountInfo{Target: target, ReadOnly: readOnly}
		}
	}

	return result
}

// extractHCLStringField extracts a `field = "value"` string from a block of
// HCL text. Returns "" if not found.
func extractHCLStringField(block, field string) string {
	re := regexp.MustCompile(regexp.QuoteMeta(field) + `\s*=\s*"([^"]*)"`)
	m := re.FindStringSubmatch(block)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// extractHCLBoolField extracts a `field = true/false` boolean from a block of
// HCL text.
func extractHCLBoolField(block, field string) bool {
	re := regexp.MustCompile(regexp.QuoteMeta(field) + `\s*=\s*(true|false)`)
	m := re.FindStringSubmatch(block)
	return len(m) >= 2 && m[1] == "true"
}

// extractAuthFromHCL scans the raw HCL text for the first `auth {}` block for a docker-driver auth block
// inside the first config {} and extracts username/password. Used as a
// fallback when the Nomad-parsed JSON doesn't carry auth in Config.auth.
func extractAuthFromHCL(hcl string) *apiclient.RegistryAuth {
	taskBody, ok := findFirstTaskBlock(hcl)
	if !ok {
		return nil
	}
	// Search for `config {` within the task body substring.
	sub := hcl[taskBody.start:taskBody.end]
	configRe := regexp.MustCompile(`(?m)^[ \t]*config[ \t]*\{`)
	loc := configRe.FindStringIndex(sub)
	if loc == nil {
		return nil
	}
	// loc is relative to `sub`; convert back to absolute positions in `hcl`.
	braceIdx := taskBody.start + loc[1] - 1 // index of the `{`
	closeIdx, ok := matchBrace(hcl, braceIdx)
	if !ok {
		return nil
	}
	configText := hcl[taskBody.start+loc[0] : closeIdx+1]

	authRe := regexp.MustCompile(`(?ms)^[ \t]*auth[ \t]*\{.*?\n[ \t]*\}`)
	authMatch := authRe.FindString(configText)
	if authMatch == "" {
		return nil
	}
	uRe := regexp.MustCompile(`(?m)^[ \t]*username[ \t]*=[ \t]*"([^"]*)"`)
	pRe := regexp.MustCompile(`(?m)^[ \t]*password[ \t]*=[ \t]*"([^"]*)"`)
	uMatch := uRe.FindStringSubmatch(authMatch)
	pMatch := pRe.FindStringSubmatch(authMatch)
	if (len(uMatch) < 2 || uMatch[1] == "") && (len(pMatch) < 2 || pMatch[1] == "") {
		return nil
	}
	auth := &apiclient.RegistryAuth{}
	if len(uMatch) >= 2 {
		auth.Username = uMatch[1]
	}
	if len(pMatch) >= 2 {
		auth.Password = pMatch[1]
	}
	return auth
}

// detectFieldIndent returns the leading whitespace of the first line in
// `block` that contains a field assignment (`key = value`). Lines that are
// comments, block headers (ending with `{`), or closing braces are skipped.
// Used so multi-line block replacements match the indentation of their
// sibling fields rather than the block header's indentation.
func detectFieldIndent(block string) string {
	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Only consider field assignments, not block headers or braces.
		if !strings.Contains(trimmed, "=") {
			continue
		}
		return line[:len(line)-len(trimmed)]
	}
	return "  "
}

// --- block emitters ---

func emitEnvBlock(env []apiclient.KeyValue) string {
	if len(env) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("      env {\n")
	for _, kv := range env {
		fmt.Fprintf(&b, "        %s = %s\n", hclKey(kv.Key), hclQuoted(kv.Value))
	}
	b.WriteString("      }\n")
	return b.String()
}

func emitResourcesBlock(memory, cpus, cpuType string) string {
	if memory == "" && cpus == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("      resources {\n")
	if memory != "" {
		memMB, _ := memoryToMB(memory)
		fmt.Fprintf(&b, "        memory = %d\n", memMB)
	}
	if cpus != "" {
		// Emit as integer — both cpu (MHz) and cores are integers in Nomad.
		val, _ := strconv.ParseInt(cpus, 10, 64)
		if cpuType == "cores" {
			fmt.Fprintf(&b, "        cores = %d\n", val)
		} else {
			// Default to cpu (MHz) — the common case for most Nomad jobs.
			fmt.Fprintf(&b, "        cpu = %d\n", val)
		}
	}
	b.WriteString("      }\n")
	return b.String()
}

// storageVolumes filters the Storage list to entries with Kind "volume" that
// have a Name. These are the entries that expand to Nomad group-level
// `volume "name" {}` stanzas + Volume Definition entries. "bind" entries are
// excluded — they only emit `volume_mount` references to volumes declared
// elsewhere (by a "volume" entry or pre-existing in Nomad).
// Deduplicates by normalised Name so that `${{ .space.name }}` and
// `${{.space.name}}` are treated as the same volume.
func storageVolumes(entries []apiclient.StorageEntry) []apiclient.StorageEntry {
	seen := map[string]bool{}
	var out []apiclient.StorageEntry
	for _, e := range entries {
		if e.Kind == "volume" && e.Name != "" {
			key := normaliseVarSpacing(e.Name)
			if !seen[key] {
				seen[key] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// normaliseVarSpacing collapses whitespace inside ${{ ... }} variable
// references so that `${{ .space.name }}` and `${{.space.name}}` compare
// equal. Used for volume name deduplication only — the original spacing is
// preserved in emitted output.
func normaliseVarSpacing(s string) string {
	// Replace ${{ <optional spaces> with ${{
	// and <optional spaces> }} with }}
	result := strings.ReplaceAll(s, "${{ ", "${{")
	result = strings.ReplaceAll(result, " }}", "}}")
	// Handle multiple spaces
	for strings.Contains(result, "${{ ") || strings.Contains(result, " }}") {
		result = strings.ReplaceAll(result, "${{ ", "${{")
		result = strings.ReplaceAll(result, " }}", "}}")
	}
	return result
}

// storageEntryVolumeName returns the volume reference name for a storage entry:
// Name for "volume" kind, Name-or-HostPath for "bind" kind.
func storageEntryVolumeName(se apiclient.StorageEntry) string {
	switch se.Kind {
	case "volume":
		return se.Name
	case "bind":
		if se.Name != "" {
			return se.Name
		}
		return se.HostPath
	}
	return ""
}

// volumeAccessMode returns the access mode for a storage entry, defaulting to
// "single-node-writer" when AccessModes is empty.
func volumeAccessMode(se apiclient.StorageEntry) string {
	if len(se.AccessModes) > 0 && se.AccessModes[0].AccessMode != "" {
		return se.AccessModes[0].AccessMode
	}
	return "single-node-writer"
}

// volumeAttachmentMode returns the attachment mode for a storage entry,
// defaulting to "file-system" when AccessModes is empty.
func volumeAttachmentMode(se apiclient.StorageEntry) string {
	if len(se.AccessModes) > 0 && se.AccessModes[0].AttachmentMode != "" {
		return se.AccessModes[0].AttachmentMode
	}
	return "file-system"
}

func emitVolumeMountBlocksFromStorage(entries []apiclient.StorageEntry) string {
	var b strings.Builder
	for _, se := range entries {
		volName := storageEntryVolumeName(se)
		if volName == "" || se.ContainerPath == "" {
			continue
		}
		b.WriteString("      volume_mount {\n")
		fmt.Fprintf(&b, "        volume      = %q\n", volName)
		fmt.Fprintf(&b, "        destination = %q\n", se.ContainerPath)
		if se.ReadOnly {
			b.WriteString("        read_only   = true\n")
		}
		b.WriteString("      }\n")
	}
	return b.String()
}

func emitVolumeStanzasFromStorage(vols []apiclient.StorageEntry) string {
	if len(vols) == 0 {
		return ""
	}
	var b strings.Builder
	for _, se := range vols {
		volName := storageEntryVolumeName(se)
		if volName == "" {
			continue
		}
		vt := se.VolumeType
		if vt == "" {
			if se.Kind == "bind" || strings.EqualFold(se.PluginID, "mkdir") {
				vt = "host"
			} else {
				vt = "csi"
			}
		}
		source := volName
		if se.Kind == "bind" && se.HostPath != "" {
			source = se.HostPath
		}
		fmt.Fprintf(&b, "    volume %q {\n", volName)
		fmt.Fprintf(&b, "      type            = %q\n", vt)
		fmt.Fprintf(&b, "      source          = %q\n", source)
		if vt != "host" {
			fmt.Fprintf(&b, "      attachment_mode = %q\n", volumeAttachmentMode(se))
			fmt.Fprintf(&b, "      access_mode     = %q\n", volumeAccessMode(se))
		}
		if se.ReadOnly {
			b.WriteString("      read_only       = true\n")
		}
		b.WriteString("    }\n")
	}
	return b.String()
}

// emitServiceBlocks generates one Nomad service {} block per port so Consul
// can discover and route to it. Each service references its port label.
func emitServiceBlocks(ports []apiclient.PortMapping) string {
	if len(ports) == 0 {
		return ""
	}
	var b strings.Builder
	for _, p := range ports {
		label := p.Label
		if label == "" {
			label = protocolToLabel(p.Protocol)
		}
		// Consul service names use hyphens, not underscores.
		serviceName := strings.ReplaceAll(label, "_", "-")
		protos := expandProtocols(p.Protocol)
		if len(protos) <= 1 {
			b.WriteString("      service {\n")
			fmt.Fprintf(&b, "        name = %q\n", serviceName)
			fmt.Fprintf(&b, "        port = %q\n", label)
			b.WriteString("      }\n")
		} else {
			for _, proto := range protos {
				b.WriteString("      service {\n")
				fmt.Fprintf(&b, "        name = %q\n", serviceName+"-"+proto)
				fmt.Fprintf(&b, "        port = %q\n", label+"-"+proto)
				b.WriteString("      }\n")
			}
		}
	}
	return b.String()
}

// emitTemplateBlocks produces the `template {}` blocks that go at the task
// level. Each block carries its heredoc data, destination, and optional
// change_mode/change_signal.
func emitTemplateBlocks(templates []apiclient.NomadTemplate) string {
	if len(templates) == 0 {
		return ""
	}
	var b strings.Builder
	for _, t := range templates {
		if t.Destination == "" {
			continue
		}
		b.WriteString("      template {\n")
		b.WriteString("        data = <<EOF\n")
		b.WriteString(t.Data)
		if !strings.HasSuffix(t.Data, "\n") {
			b.WriteString("\n")
		}
		b.WriteString("EOF\n")
		fmt.Fprintf(&b, "        destination = %q\n", t.Destination)
		if t.ChangeMode != "" {
			fmt.Fprintf(&b, "        change_mode = %q\n", t.ChangeMode)
		}
		if t.ChangeSignal != "" {
			fmt.Fprintf(&b, "        change_signal = %q\n", t.ChangeSignal)
		}
		b.WriteString("      }\n")
	}
	return b.String()
}

// emitConfigMountBlocks produces the `mount {}` blocks that go inside the
// docker-driver `config {}` for templates that have a mount target set.
// Only templates with MountTarget produce a mount block.
func emitConfigMountBlocks(templates []apiclient.NomadTemplate) string {
	var b strings.Builder
	for _, t := range templates {
		if t.MountTarget == "" {
			continue
		}
		b.WriteString("        mount {\n")
		b.WriteString("          type     = \"bind\"\n")
		fmt.Fprintf(&b, "          source   = %q\n", t.Destination)
		fmt.Fprintf(&b, "          target   = %q\n", t.MountTarget)
		if t.MountReadonly {
			b.WriteString("          readonly = true\n")
		}
		b.WriteString("        }\n")
	}
	return b.String()
}

// replaceTemplateMounts replaces mount blocks inside the config block whose
// `source` starts with "local/" (template destinations). Non-template mounts
// (host paths, conditional mounts) are left untouched.
func replaceTemplateMounts(configBlock string, templates []apiclient.NomadTemplate) string {
	// Remove all existing local/ mounts regardless of which template they
	// belonged to — old mounts from removed templates must be cleaned up too.
	configBlock = removeMountsBySourcePrefix(configBlock, "local/")

	newMounts := emitConfigMountBlocks(templates)
	if newMounts == "" {
		return configBlock
	}
	return insertAfterBrace(configBlock, newMounts)
}

// removeMountsBySourcePrefix removes all mount blocks whose source starts
// with the given prefix. Used to clean up template mounts when no templates
// remain in the spec.
func removeMountsBySourcePrefix(configBlock, prefix string) string {
	mountRe := regexp.MustCompile(`(?m)^[ \t]*mount\b[^{\n"]*(?:"[^"]*"[^{\n"]*)*\{`)
	locs := mountRe.FindAllStringIndex(configBlock, -1)
	if len(locs) == 0 {
		return configBlock
	}
	srcRe := regexp.MustCompile(`source\s*=\s*"([^"]*)"`)
	for i := len(locs) - 1; i >= 0; i-- {
		open := locs[i][1] - 1
		closeIdx, ok := matchBrace(configBlock, open)
		if !ok {
			continue
		}
		blockText := configBlock[locs[i][0] : closeIdx+1]
		m := srcRe.FindStringSubmatch(blockText)
		if m != nil && strings.HasPrefix(m[1], prefix) {
			end := closeIdx + 1
			if end < len(configBlock) && configBlock[end] == '\n' {
				end++
			}
			configBlock = configBlock[:locs[i][0]] + configBlock[end:]
		}
	}
	return configBlock
}

// replaceTemplateVolumes handles the podman driver's `volumes = [...]` list
// inside config. Entries whose host path starts with "local/" (template
// destinations) are removed and re-emitted from the spec's templates.
// Non-template volume entries are preserved.
func replaceTemplateVolumes(configBlock string, templates []apiclient.NomadTemplate) string {
	// Build new template volume entries in "source:target[:ro]" format.
	var newEntries []string
	for _, t := range templates {
		if t.MountTarget == "" || t.Destination == "" {
			continue
		}
		entry := t.Destination + ":" + t.MountTarget
		if t.MountReadonly {
			entry += ":ro"
		}
		newEntries = append(newEntries, fmt.Sprintf("%q", entry))
	}

	// Find volumes = [...] field.
	volRe := regexp.MustCompile(`(?ms)^\s*volumes\s*=\s*\[([^\]]*)\]`)
	loc := volRe.FindStringSubmatchIndex(configBlock)

	if loc == nil {
		if len(newEntries) == 0 {
			return configBlock
		}
		volLiteral := fmt.Sprintf("        volumes = [%s]\n", strings.Join(newEntries, ", "))
		return insertAfterBrace(configBlock, volLiteral)
	}

	// Parse existing entries, keeping only non-template ones.
	existingContent := configBlock[loc[2]:loc[3]]
	entryRe := regexp.MustCompile(`"([^"]*)"`)
	var keptEntries []string
	for _, m := range entryRe.FindAllStringSubmatch(existingContent, -1) {
		parts := strings.SplitN(m[1], ":", 3)
		if len(parts) >= 1 && strings.HasPrefix(parts[0], "local/") {
			continue
		}
		keptEntries = append(keptEntries, fmt.Sprintf("%q", m[1]))
	}

	allEntries := append(keptEntries, newEntries...)

	if len(allEntries) == 0 {
		// Remove the entire volumes field + its line.
		start := loc[0]
		for start > 0 && configBlock[start-1] != '\n' {
			start--
		}
		end := loc[1]
		if end < len(configBlock) && configBlock[end] == '\n' {
			end++
		}
		return configBlock[:start] + configBlock[end:]
	}

	// Rebuild, preserving the original line's indentation.
	lineStart := loc[0]
	for lineStart > 0 && configBlock[lineStart-1] != '\n' {
		lineStart--
	}
	indent := configBlock[lineStart:loc[0]]
	replacement := indent + "volumes = [" + strings.Join(allEntries, ", ") + "]"
	return configBlock[:lineStart] + replacement + configBlock[loc[1]:]
}

func emitNetworkBlock(ports []apiclient.PortMapping, mode string) string {
	if len(ports) == 0 && mode == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("    network {\n")
	// Only write a mode when one is known. Defaulting to bridge here would
	// silently change the networking of any job that relies on the driver's
	// default (or on a mode set elsewhere).
	if mode != "" {
		fmt.Fprintf(&b, "      mode = %q\n", mode)
	}
	for _, p := range ports {
		label := p.Label
		if label == "" {
			label = protocolToLabel(p.Protocol)
		}
		protos := expandProtocols(p.Protocol)
		if len(protos) <= 1 {
			fmt.Fprintf(&b, "      port %q {\n", label)
			fmt.Fprintf(&b, "        to = %d\n", p.ContainerPort)
			if p.HostPort > 0 {
				fmt.Fprintf(&b, "        static = %d\n", p.HostPort)
			}
			b.WriteString("      }\n")
		} else {
			// Multi-protocol (tcp+udp): emit one block per protocol with a
			// suffix so labels stay unique within the job.
			for _, proto := range protos {
				fmt.Fprintf(&b, "      port %q {\n", label+"-"+proto)
				fmt.Fprintf(&b, "        to = %d\n", p.ContainerPort)
				if p.HostPort > 0 {
					fmt.Fprintf(&b, "        static = %d\n", p.HostPort)
				}
				b.WriteString("      }\n")
			}
		}
	}
	b.WriteString("    }\n")
	return b.String()
}

// emitConfigPortsLine returns the `ports = [...]` literal that lives inside a
// docker-driver config {} block, listing the network port labels the task
// binds to. Returns empty when there are no ports — callers should remove the
// existing line in that case.
func emitConfigPortsLine(ports []apiclient.PortMapping) string {
	literal := configPortsLiteral(ports)
	if literal == "" {
		return ""
	}
	return fmt.Sprintf("  ports = %s\n", literal)
}

// configPortsLiteral returns just the bracketed list of port labels (or an
// empty string when there are no ports).
func configPortsLiteral(ports []apiclient.PortMapping) string {
	if len(ports) == 0 {
		return ""
	}
	var labels []string
	for _, p := range ports {
		l := p.Label
		if l == "" {
			l = protocolToLabel(p.Protocol)
		}
		protos := expandProtocols(p.Protocol)
		if len(protos) <= 1 {
			labels = append(labels, l)
		} else {
			for _, proto := range protos {
				labels = append(labels, l+"-"+proto)
			}
		}
	}
	return hclList(labels)
}

// --- string formatting helpers ---

func hclQuoted(s string) string {
	return fmt.Sprintf("%q", s)
}

// hclKey sanitises an env var key for HCL. HCL identifiers must match
// [A-Za-z_][A-Za-z0-9_-]*; anything else is quoted.
func hclKey(k string) string {
	if k == "" {
		return `""`
	}
	for i, r := range k {
		if i == 0 && !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_') {
			return fmt.Sprintf("%q", k)
		}
		if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
			return fmt.Sprintf("%q", k)
		}
	}
	return k
}

func hclList(items []string) string {
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = fmt.Sprintf("%q", s)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// formatMemory converts MB to a human-friendly string (e.g. 2048 -> "2G").
func formatMemory(mb int64) string {
	if mb <= 0 {
		return ""
	}
	if mb%1024 == 0 {
		return fmt.Sprintf("%dG", mb/1024)
	}
	return fmt.Sprintf("%dM", mb)
}

// memoryToMB parses strings like "2G", "512M", "1024" into MB.
func memoryToMB(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	last := s[len(s)-1]
	switch last {
	case 'g', 'G':
		v, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		return v * 1024, err
	case 'm', 'M':
		v, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		return v, err
	case 'k', 'K':
		v, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		return v / 1024, err
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	// Assume bytes.
	return v / (1024 * 1024), nil
}

func interfaceSliceToStrings(in []interface{}) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// toStringSlice coerces a value from parsed JSON into a string slice. Nomad's
// parse endpoint yields []interface{}, but a driver config decoded from other
// sources (or a test fake) may hand back []string.
func toStringSlice(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		return interfaceSliceToStrings(x)
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		return []string{x}
	}
	return nil
}

func getStr(m map[string]interface{}, key string) string {
	s, _ := m[key].(string)
	return s
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

func toInt(v interface{}) (int, bool) {
	f, ok := toFloat(v)
	if !ok {
		return 0, false
	}
	return int(f), true
}

// labelToProtocol guesses a protocol ("tcp"/"udp") from a Nomad port label.
func labelToProtocol(label string) string {
	switch strings.ToLower(label) {
	case "udp":
		return "udp"
	case "http", "https":
		return "tcp"
	}
	if strings.HasSuffix(strings.ToLower(label), "-udp") {
		return "udp"
	}
	return "tcp"
}

func protocolToLabel(p string) string {
	switch strings.ToLower(p) {
	case "udp":
		return "udp"
	case "":
		return "http"
	}
	return strings.ToLower(p)
}

// ---------------------------------------------------------------------------
// Nomad volume definitions (Volume Definition YAML for Nomad platform)
// ---------------------------------------------------------------------------

// nomadVolumeSpec mirrors model.CSIVolumes + managed paths for parse/build.
type nomadVolumeSpec struct {
	Volumes []nomadVolumeEntry `yaml:"volumes,omitempty"`
	Paths   []string           `yaml:"paths,omitempty"`
}

type nomadVolumeEntry struct {
	ID           string             `yaml:"id,omitempty"`
	Name         string             `yaml:"name"`
	Namespace    string             `yaml:"namespace,omitempty"`
	PluginID     string             `yaml:"plugin_id"`
	Type         string             `yaml:"type"`
	MountOptions *nomadMountOptions `yaml:"mount_options,omitempty"`
	CapacityMin  string             `yaml:"capacity_min,omitempty"`
	CapacityMax  string             `yaml:"capacity_max,omitempty"`
	Capabilities []nomadCapability  `yaml:"capabilities,omitempty"`
	Secrets      map[string]string  `yaml:"secrets,omitempty"`
	Parameters   map[string]string  `yaml:"parameters,omitempty"`
}

type nomadMountOptions struct {
	FsType     string   `yaml:"fs_type,omitempty"`
	MountFlags []string `yaml:"mount_flags,omitempty"`
}

type nomadCapability struct {
	AccessMode     string `yaml:"access_mode,omitempty"`
	AttachmentMode string `yaml:"attachment_mode,omitempty"`
}

// parseNomadVolumeDefinitionsToStorage parses the Volume Definition YAML into
// StorageEntry rows (Kind "volume" for each volume, Kind "path" for each path).
func parseNomadVolumeDefinitionsToStorage(volumes string) []apiclient.StorageEntry {
	if strings.TrimSpace(volumes) == "" {
		return nil
	}
	var nvs nomadVolumeSpec
	if err := yaml.Unmarshal([]byte(volumes), &nvs); err != nil {
		return nil
	}
	if len(nvs.Volumes) == 0 && len(nvs.Paths) == 0 {
		return nil
	}
	var entries []apiclient.StorageEntry
	for _, v := range nvs.Volumes {
		se := apiclient.StorageEntry{
			Kind:        "volume",
			Name:        v.Name,
			VolumeType:  v.Type,
			PluginID:    v.PluginID,
			CapacityMin: v.CapacityMin,
			CapacityMax: v.CapacityMax,
			Namespace:   v.Namespace,
			Secrets:     v.Secrets,
			Parameters:  v.Parameters,
		}
		if v.MountOptions != nil {
			se.FsType = v.MountOptions.FsType
			se.MountFlags = v.MountOptions.MountFlags
		}
		for _, cap := range v.Capabilities {
			se.AccessModes = append(se.AccessModes, apiclient.VolumeCapability{
				AccessMode:     cap.AccessMode,
				AttachmentMode: cap.AttachmentMode,
			})
		}
		entries = append(entries, se)
	}
	for _, p := range nvs.Paths {
		entries = append(entries, apiclient.StorageEntry{Kind: "path", HostPath: p})
	}
	return entries
}

// mergeNomadStorage merges the mounts extracted from the job HCL (already in
// spec.Storage as tentative "bind" entries with HostPath = volume name) with
// the volume definitions parsed from the Volume Definition YAML. A mount whose
// HostPath matches a definition's Name is reclassified as Kind "volume" and
// gets the definition's CSI fields. Definitions without a matching mount are
// appended as unmounted volumes (ContainerPath empty). Mounts with no matching
// definition stay as "bind" (ad-hoc references to external volumes).
func mergeNomadStorage(mounts []apiclient.StorageEntry, volumesYAML string) []apiclient.StorageEntry {
	defs := parseNomadVolumeDefinitionsToStorage(volumesYAML)
	if len(defs) == 0 && len(mounts) == 0 {
		return nil
	}

	defsByName := map[string]*apiclient.StorageEntry{}
	for i := range defs {
		if defs[i].Kind == "volume" && defs[i].Name != "" {
			defsByName[normaliseVarSpacing(defs[i].Name)] = &defs[i]
		}
	}

	usedDef := map[string]bool{}
	var result []apiclient.StorageEntry

	for _, m := range mounts {
		key := normaliseVarSpacing(m.HostPath)
		if def, ok := defsByName[key]; ok {
			usedDef[key] = true
			merged := *def
			merged.ContainerPath = m.ContainerPath
			merged.ReadOnly = m.ReadOnly
			result = append(result, merged)
		} else {
			result = append(result, m) // stays as "bind"
		}
	}

	// Append definitions without a matching mount (unmounted volumes + paths).
	for _, d := range defs {
		switch d.Kind {
		case "volume":
			key := normaliseVarSpacing(d.Name)
			if key != "" && !usedDef[key] {
				result = append(result, d)
			}
		case "path":
			result = append(result, d)
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// buildNomadStorageDefinitions renders the Volume Definition YAML for the
// Nomad platform from the wizard's Storage entries. Only "volume" and "path"
// kinds produce definition entries; "bind" entries don't (they reference
// volumes declared elsewhere). When originalVolumes is non-empty and
// parseable, a comment-preserving patcher is attempted.
func buildNomadStorageDefinitions(entries []apiclient.StorageEntry, originalVolumes string) string {
	if len(entries) == 0 {
		return originalVolumes
	}

	nvs := nomadVolumeSpec{}
	for _, e := range entries {
		switch e.Kind {
		case "volume":
			if e.Name == "" {
				continue
			}
			entry := nomadVolumeEntry{
				ID:          e.Name, // ID defaults to Name
				Name:        e.Name,
				Namespace:   e.Namespace,
				PluginID:    e.PluginID,
				Type:        e.VolumeType,
				CapacityMin: e.CapacityMin,
				CapacityMax: e.CapacityMax,
				Secrets:     e.Secrets,
				Parameters:  e.Parameters,
			}
			if entry.Type == "" {
				if strings.EqualFold(e.PluginID, "mkdir") {
					entry.Type = "host"
				} else {
					entry.Type = "csi"
				}
			}
			if e.FsType != "" || len(e.MountFlags) > 0 {
				entry.MountOptions = &nomadMountOptions{
					FsType:     e.FsType,
					MountFlags: e.MountFlags,
				}
			}
			for _, am := range e.AccessModes {
				entry.Capabilities = append(entry.Capabilities, nomadCapability{
					AccessMode:     am.AccessMode,
					AttachmentMode: am.AttachmentMode,
				})
			}
			if len(entry.Capabilities) == 0 && entry.Type == "csi" {
				entry.Capabilities = []nomadCapability{{
					AccessMode:     "single-node-writer",
					AttachmentMode: "file-system",
				}}
			}
			nvs.Volumes = append(nvs.Volumes, entry)
		case "path":
			if e.HostPath == "" {
				continue
			}
			nvs.Paths = append(nvs.Paths, e.HostPath)
		}
	}

	if len(nvs.Volumes) == 0 && len(nvs.Paths) == 0 {
		return originalVolumes
	}

	out, err := yaml.Marshal(nvs)
	if err != nil {
		return originalVolumes
	}
	return string(out)
}
