package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// Feature catalog: every product area the integration suite aims to cover.
// The report lists any area with no recorded results as a gap, so missing
// coverage is visible rather than silent. Pro-only areas are marked and stay
// untested in the OSS repo (they light up in the pro fork).
var featureCatalog = []string{
	"server-info",
	"auth",
	"auth-rate-limiting",
	"totp",
	"users",
	"roles",
	"groups",
	"permissions",
	"quotas",
	"tokens",
	"templates",
	"spaces",
	"space-lifecycle",
	"space-stacks",
	"run-command",
	"files",
	"logs",
	"sharing",
	"transfer",
	"volumes",
	"pools",
	"scripts",
	"skills",
	"commands",
	"stack-definitions",
	"template-vars",
	"events",
	"event-sinks",
	"audit-log",
	"mcp",
	"search",
	"port-forwarding",
	"tunnels",
	"usage-history",
	"chat",
	"cluster-gossip",
	"leaf-node",
	// Pro areas (exercised only in the pro fork build):
	"pro:detection",
	"pro:log-spool",
	"pro:space-log-forwarding",
	"pro:log-sinks",
	"pro:vault-secrets",
	"pro:user-activity",
	"pro:user-access",
	"pro:peermesh",
	"pro:migration",
}

type result struct {
	Feature  string
	Test     string
	Status   string // pass | fail | skip
	Duration time.Duration
	Detail   string
}

var (
	resultsMu sync.Mutex
	results   []result
	// phase describes what the suite is currently doing; shown in the live
	// report header so a watcher can see progress during long setups.
	phase = "starting"
)

// Progress updates the current phase note and refreshes the on-disk report,
// so `tail -f build/test/integration-report.md` (or just re-opening it)
// shows what the suite is doing even while a single long test runs.
func Progress(note string) {
	resultsMu.Lock()
	phase = note
	writeReportLocked()
	resultsMu.Unlock()
}

// Feature tags the test with a feature area and records its outcome when the
// test finishes. Call it first in every test function.
func Feature(t *testing.T, feature string) *testing.T {
	start := time.Now()
	t.Cleanup(func() {
		status := "pass"
		switch {
		case t.Failed():
			status = "fail"
		case t.Skipped():
			status = "skip"
		}
		resultsMu.Lock()
		results = append(results, result{
			Feature:  feature,
			Test:     t.Name(),
			Status:   status,
			Duration: time.Since(start),
		})
		phase = fmt.Sprintf("finished %s", t.Name())
		writeReportLocked()
		resultsMu.Unlock()
	})
	return t
}

// ReportSummary returns one-line pass/fail/skip counts.
func ReportSummary() (pass, fail, skip int) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "skip":
			skip++
		}
	}
	return
}

// WriteReport renders the final feature matrix as markdown and returns the
// report path. Features present in the catalog but without results are
// listed as "no tests" gaps (pro: prefixed ones are expected gaps in the
// OSS build).
func WriteReport(extra string) (string, error) {
	resultsMu.Lock()
	defer resultsMu.Unlock()
	reportExtra = extra
	phase = "complete"
	return writeReportLocked(), nil
}

var reportExtra string

// writeReportLocked renders and writes the report; callers hold resultsMu.
func writeReportLocked() string {
	byFeature := map[string][]result{}
	for _, r := range results {
		byFeature[r.Feature] = append(byFeature[r.Feature], r)
	}

	features := make([]string, 0, len(byFeature))
	for f := range byFeature {
		features = append(features, f)
	}
	sort.Strings(features)

	var b strings.Builder
	pass, fail, skip := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			pass++
		case "fail":
			fail++
		case "skip":
			skip++
		}
	}

	fmt.Fprintf(&b, "# knot integration test report\n\n")
	fmt.Fprintf(&b, "Status: **%s** — updated %s\n\n", phase, time.Now().Format("15:04:05"))
	fmt.Fprintf(&b, "Generated: %s\n\n", time.Now().Format(time.RFC3339))
	if reportExtra != "" {
		fmt.Fprintf(&b, "%s\n\n", reportExtra)
	}
	fmt.Fprintf(&b, "**%d passed, %d failed, %d skipped**\n\n", pass, fail, skip)

	statusIcon := func(s string) string {
		switch s {
		case "pass":
			return "✅"
		case "fail":
			return "❌"
		default:
			return "⏭️"
		}
	}

	b.WriteString("| Feature | Description | Status | Tests (pass/fail/skip) |\n|---|---|---|---|\n")
	for _, feature := range featureCatalog {
		if strings.HasPrefix(feature, "pro:") && !ProBuild {
			continue // pro feature areas only appear in the pro build's report
		}
		rs, ok := byFeature[feature]
		if !ok || len(rs) == 0 {
			note := "no tests"
			if strings.HasPrefix(feature, "pro:") {
				note = "pro only — not yet tested"
			}
			fmt.Fprintf(&b, "| `%s` | %s | ⚪ %s | — |\n", feature, featureDescription(feature), note)
			continue
		}
		p, f, s := 0, 0, 0
		for _, r := range rs {
			switch r.Status {
			case "pass":
				p++
			case "fail":
				f++
			case "skip":
				s++
			}
		}
		status := "✅"
		if f > 0 {
			status = "❌"
		} else if p == 0 {
			status = "⏭️"
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %d/%d/%d |\n", feature, featureDescription(feature), status, p, f, s)
	}
	// Any recorded feature not in the catalog (typos, new areas).
	for _, feature := range features {
		if _, ok := containsFeature(featureCatalog, feature); !ok {
			rs := byFeature[feature]
			p, f, s := 0, 0, 0
			for _, r := range rs {
				switch r.Status {
				case "pass":
					p++
				case "fail":
					f++
				case "skip":
					s++
				}
			}
			fmt.Fprintf(&b, "| `%s` (uncatalogued) | ⚠️ | %d/%d/%d |\n", feature, p, f, s)
		}
	}

	b.WriteString("\n## Detail\n\n")
	for _, feature := range featureCatalog {
		if strings.HasPrefix(feature, "pro:") && !ProBuild {
			continue
		}
		rs, ok := byFeature[feature]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", feature)
		for _, r := range rs {
			fmt.Fprintf(&b, "- %s `%s` (%.1fs)\n", statusIcon(r.Status), r.Test, r.Duration.Seconds())
		}
		b.WriteString("\n")
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		repoRoot = "."
	}
	dir := filepath.Join(repoRoot, buildDir)
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "integration-report.md")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(b.String()), 0o644); err != nil {
		return ""
	}
	os.Rename(tmp, path) // atomic-ish refresh for tail -f watchers
	return path
}

// featureDescriptions gives each catalog area a plain-language name for the
// final report.
var featureDescriptions = map[string]string{
	"server-info":               "Server health, ping, info & cluster nodes",
	"auth":                      "Login, logout & session lifecycle",
	"auth-rate-limiting":        "Failed-login rate limiting and admin flush",
	"totp":                      "TOTP second-factor login",
	"users":                     "User CRUD",
	"roles":                     "Role CRUD & permission resolution",
	"groups":                    "Group CRUD",
	"permissions":               "Permission enforcement & space isolation",
	"quotas":                    "Per-user space quotas",
	"tokens":                    "API tokens, scopes & revocation",
	"templates":                 "Template CRUD, export & import",
	"spaces":                    "Spaces (creation & dev URL routing)",
	"space-lifecycle":           "Space start/stop/restart/update",
	"space-stacks":              "Stacks of spaces (start/stop/delete)",
	"run-command":               "Command execution inside spaces",
	"files":                     "File read/write/grep/find/sed/edit",
	"logs":                      "Syslog from spaces to the log stream",
	"sharing":                   "Space sharing between users",
	"transfer":                  "Space ownership transfer",
	"volumes":                   "Volume CRUD",
	"pools":                     "Space pools (scale up/down)",
	"scripts":                   "Scripts CRUD",
	"skills":                    "Skills CRUD",
	"commands":                  "Slash commands CRUD",
	"stack-definitions":         "Stack definitions & validation",
	"template-vars":             "Template variables",
	"events":                    "Server-sent events stream",
	"event-sinks":               "Event sinks (webhook delivery)",
	"audit-log":                 "Audit log & filtering",
	"mcp":                       "MCP servers CRUD",
	"search":                    "Global search",
	"port-forwarding":           "Port forwards & throttling",
	"tunnels":                   "Space web tunnels",
	"usage-history":             "Space usage history samples",
	"chat":                      "AI chat (OpenAI-compatible endpoint)",
	"cluster-gossip":            "Gossip cluster: 2 nodes, config replication",
	"leaf-node":                 "Leaf node against an origin server",
	"pro:detection":             "Pro: anomaly detection on audit events",
	"pro:log-spool":             "Pro: log spool & replay via VictoriaLogs",
	"pro:space-log-forwarding":  "Pro: space logs forwarded to log output",
	"pro:log-sinks":             "Pro: per-user log sinks (isolation)",
	"pro:vault-secrets":         "Pro: Vault secret provider in templates",
	"pro:user-activity":         "Pro: user activity tracking",
	"pro:user-access":           "Pro: user access overview",
	"pro:peermesh":              "Pro: direct agent-to-agent port forwards",
	"pro:migration":             "Pro: failed-node space migration",
}

func featureDescription(feature string) string {
	if d, ok := featureDescriptions[feature]; ok {
		return d
	}
	return "—"
}

func containsFeature(haystack []string, needle string) (int, bool) {
	for i, s := range haystack {
		if s == needle {
			return i, true
		}
	}
	return -1, false
}
