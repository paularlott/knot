package container

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

var portEnvVarNames = []string{"KNOT_HTTP_PORT", "KNOT_HTTPS_PORT", "KNOT_TCP_PORT"}

// EnvVar is a single environment variable as an ordered key/value pair.
type EnvVar struct {
	Key   string
	Value string
}

// ParseEnvStrings parses "KEY=value" entries into an ordered slice of EnvVar.
// Entries without "=" or with an empty key are skipped. Order is preserved so
// callers that care about insertion order (e.g. the spec wizard, which
// round-trips the user's ordering through HCL/YAML emission) don't lose it.
func ParseEnvStrings(env []string) []EnvVar {
	out := make([]EnvVar, 0, len(env))
	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		out = append(out, EnvVar{Key: parts[0], Value: parts[1]})
	}
	return out
}

// FormatEnvStrings formats env vars back to "KEY=value" strings, preserving
// the order of the input slice.
func FormatEnvStrings(env []EnvVar) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		out = append(out, e.Key+"="+e.Value)
	}
	return out
}

// RemoveExistingPortEnvVars removes any existing KNOT_HTTP_PORT, KNOT_HTTPS_PORT,
// or KNOT_TCP_PORT entries from the given environment slice.
func RemoveExistingPortEnvVars(env []string) []string {
	filtered := env[:0]
	for _, e := range env {
		key := strings.SplitN(e, "=", 2)[0]
		drop := false
		for _, pk := range portEnvVarNames {
			if key == pk {
				drop = true
				break
			}
		}
		if !drop {
			filtered = append(filtered, e)
		}
	}
	return filtered
}

func BuildPortEnvVars(template *model.Template) []string {
	var httpPorts, httpsPorts, tcpPorts []string
	for _, p := range template.Ports {
		entry := fmt.Sprintf("%d=%s", p.Port, p.Name)
		switch p.Protocol {
		case "http":
			httpPorts = append(httpPorts, entry)
		case "https":
			httpsPorts = append(httpsPorts, entry)
		case "tcp":
			tcpPorts = append(tcpPorts, entry)
		}
	}

	var env []string
	if len(httpPorts) > 0 {
		env = append(env, "KNOT_HTTP_PORT="+strings.Join(httpPorts, ","))
	}
	if len(httpsPorts) > 0 {
		env = append(env, "KNOT_HTTPS_PORT="+strings.Join(httpsPorts, ","))
	}
	if len(tcpPorts) > 0 {
		env = append(env, "KNOT_TCP_PORT="+strings.Join(tcpPorts, ","))
	}
	return env
}
