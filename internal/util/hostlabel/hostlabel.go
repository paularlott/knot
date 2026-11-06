// Package hostlabel makes the first label of a space's web hostnames,
// <user>--<name>--<port>, fit the 63 characters DNS allows a label: a
// longer label is never resolved, so the space could not be reached.
//
// A label that fits is the names as they are, lowercased. One that does
// not keeps as much of the space's, alt name's or pool's name as fits,
// then a hyphen and a hash of the whole name, which the proxy matches the
// name by.
package hostlabel

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// MaxLen is the most characters DNS allows in one label.
const MaxLen = 63

// hashLen is the length of the hash that ends a shortened name.
const hashLen = 8

// Label is the label of the host for a user's space, alt name or pool and
// a port, a number or "vnc".
func Label(user, name, port string) string {
	user, name, port = strings.ToLower(user), strings.ToLower(name), strings.ToLower(port)
	full := user + "--" + name + "--" + port
	if len(full) <= MaxLen {
		return full
	}
	return user + "--" + short(name, MaxLen-len(user)-len(port)-4) + "--" + port
}

// short is name in at most max characters: as much of it as fits, then a
// hyphen and the hash of all of it. It keeps at least the first character,
// so a very long username can still leave a label too long.
func short(name string, max int) string {
	keep := min(max-hashLen-1, len(name))
	if keep < 1 {
		keep = 1
	}
	// The prefix never ends in a hyphen, so the label never holds "--"
	// inside the name, which separates its parts.
	return strings.TrimRight(name[:keep], "-") + "-" + Hash(name)
}

// Hash is the hash that ends name shortened: FNV-1a, 32 bits, of the name
// lowercased, in hex. The web interface and the MCP tools work it out the
// same way.
func Hash(name string) string {
	h := fnv.New32a()
	h.Write([]byte(strings.ToLower(name)))
	return fmt.Sprintf("%08x", h.Sum32())
}

// Shortened says whether part, the name in a host's label, has the shape
// of a shortened name: a prefix, a hyphen and a hash.
func Shortened(part string) bool {
	i := strings.LastIndexByte(part, '-')
	if i < 1 || len(part)-i-1 != hashLen {
		return false
	}
	for _, c := range part[i+1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Matches says whether part, the name in a host's label, is name
// shortened.
func Matches(part, name string) bool {
	part, name = strings.ToLower(part), strings.ToLower(name)
	if !Shortened(part) {
		return false
	}
	i := strings.LastIndexByte(part, '-')
	return part[i+1:] == Hash(name) && strings.HasPrefix(name, part[:i])
}

// CheckTunnel refuses a web tunnel whose host label, <user>--<name>, would
// be longer than DNS allows, so no one could reach it.
func CheckTunnel(user, name string) error {
	label := strings.ToLower(user + "--" + name)
	if len(label) <= MaxLen {
		return nil
	}
	return fmt.Errorf("tunnel name %s is too long: its address starts %s, %d characters, and DNS allows %d before the first dot; use a name of at most %d characters", name, label, len(label), MaxLen, max(MaxLen-len(user)-2, 0))
}
