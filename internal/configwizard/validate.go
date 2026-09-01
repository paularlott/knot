package configwizard

import (
	"fmt"
	"strings"
)

// validateTomlConfig checks a parsed wizard-generated config for required
// fields. It returns a list of human-readable problems; empty means valid.
func validateTomlConfig(probe map[string]interface{}) []string {
	var errs []string

	server, _ := probe["server"].(map[string]interface{})
	if server == nil {
		return []string{"missing [server] section"}
	}

	str := func(key string) string {
		v, _ := server[key].(string)
		return strings.TrimSpace(v)
	}

	for _, key := range []string{"url", "agent_endpoint", "listen", "listen_agent", "timezone", "encrypt"} {
		if str(key) == "" {
			errs = append(errs, fmt.Sprintf("[server] %s is required", key))
		}
	}

	boolOf := func(sectionName, key string) bool {
		m := section(probe, sectionName)
		if m == nil {
			return false
		}
		v, _ := m[key].(bool)
		return v
	}

	// Redis alongside badger or mysql is session storage, not the primary
	// backend, so it only counts as the primary when nothing else is enabled.
	badger := boolOf("server.badgerdb", "enabled")
	mysql := boolOf("server.mysql", "enabled")
	redis := boolOf("server.redis", "enabled")

	switch {
	case badger && mysql:
		errs = append(errs, "multiple database backends enabled — pick exactly one of badgerdb, mysql, redis")
	case badger:
		if str2(probe, "server.badgerdb", "path") == "" {
			errs = append(errs, "[server.badgerdb] path is required")
		}
	case mysql:
		for _, key := range []string{"host", "user", "password", "database"} {
			if str2(probe, "server.mysql", key) == "" {
				errs = append(errs, "[server.mysql] "+key+" is required")
			}
		}
	case redis:
		if len(listOf(probe, "server.redis", "hosts")) == 0 {
			errs = append(errs, "[server.redis] hosts is required")
		}
	default:
		errs = append(errs, "no database backend enabled (one of [server.badgerdb], [server.mysql], [server.redis] is required)")
	}

	return errs
}

func section(probe map[string]interface{}, name string) map[string]interface{} {
	parts := strings.Split(name, ".")
	var cur map[string]interface{} = probe
	for _, part := range parts {
		next, _ := cur[part].(map[string]interface{})
		if next == nil {
			return nil
		}
		cur = next
	}
	return cur
}

func str2(probe map[string]interface{}, sectionName, key string) string {
	m := section(probe, sectionName)
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func listOf(probe map[string]interface{}, sectionName, key string) []interface{} {
	m := section(probe, sectionName)
	if m == nil {
		return nil
	}
	v, _ := m[key].([]interface{})
	return v
}
