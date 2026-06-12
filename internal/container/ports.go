package container

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

var portEnvVarNames = []string{"KNOT_HTTP_PORT", "KNOT_HTTPS_PORT", "KNOT_TCP_PORT"}

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
