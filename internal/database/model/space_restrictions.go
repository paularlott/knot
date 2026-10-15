package model

import (
	"slices"
	"strings"
)

// File storage modes of a restricted space.
const (
	SpaceFilesAll      = ""     // every bucket the owner can reach, as their shares allow
	SpaceFilesReadOnly = "ro"   // every bucket, read-only
	SpaceFilesNone     = "none" // no file storage at all
)

// SpaceBucketGrant is one bucket a restricted space may still reach. The id
// keeps the grant on the bucket through renames and transfers; the name is
// the bucket's full name when the grant was made, kept for reporting.
type SpaceBucketGrant struct {
	Id       string `json:"id" msgpack:"id"`
	Name     string `json:"name" msgpack:"name"`
	ReadOnly bool   `json:"read_only" msgpack:"read_only"`
}

// SpaceRestrictions narrow what a space's own credential (its agent token)
// may do, on top of what its template allows. They are applied from inside
// the space or by its owner, can only ever be tightened from inside the
// space, and hold across restarts until lifted from outside the space, or
// until a pool member goes back to its pool.
//
// Any restriction confines the space: its credential may then act only on
// the space itself, the buckets it is allowed, tunnels and port forwards,
// and may start or stop other spaces — everything else, such as making API tokens
// or running commands in other spaces, is refused, so the restrictions
// can't be stepped around.
type SpaceRestrictions struct {
	Active    bool   `json:"active" msgpack:"active"`
	NoSecrets bool   `json:"no_secrets" msgpack:"no_secrets"`
	Files     string `json:"files" msgpack:"files"`
	// Locked locks the space's processes out of everything but file storage
	// and personal secrets, which their own restrictions govern: the knot
	// command and scripts in the space may otherwise only restart or shut it
	// down and show its restrictions. Requests from outside the space, which
	// the agent carries out, such as port forwards and scripts run from the
	// desktop, still work.
	Locked bool `json:"locked" msgpack:"locked"`
	// LimitBuckets makes Buckets an allowlist: buckets not in it are hidden.
	LimitBuckets bool               `json:"limit_buckets" msgpack:"limit_buckets"`
	Buckets      []SpaceBucketGrant `json:"buckets" msgpack:"buckets"`
	// Personal secret references (Knot Pro), as prefixes that carry their
	// source's scheme, such as op://vault/. LimitSecrets makes SecretsAllow
	// an allowlist: only references under one of its prefixes may be read.
	// SecretsDeny refuses references under any of its prefixes, allowed or
	// not.
	LimitSecrets bool     `json:"limit_secrets" msgpack:"limit_secrets"`
	SecretsAllow []string `json:"secrets_allow" msgpack:"secrets_allow"`
	SecretsDeny  []string `json:"secrets_deny" msgpack:"secrets_deny"`
}

// filesRank orders the file modes from least to most strict.
var filesRank = map[string]int{SpaceFilesAll: 0, SpaceFilesReadOnly: 1, SpaceFilesNone: 2}

// ValidSpaceFilesMode reports whether mode is a known file storage mode.
func ValidSpaceFilesMode(mode string) bool {
	_, ok := filesRank[mode]
	return ok
}

// BucketAccess reports whether a restricted space may reach a bucket and,
// if so, whether only to read it. An inactive restriction allows everything.
func (r *SpaceRestrictions) BucketAccess(bucketId string) (allowed, readOnly bool) {
	if r == nil || !r.Active {
		return true, false
	}
	if r.Files == SpaceFilesNone {
		return false, false
	}
	readOnly = r.Files == SpaceFilesReadOnly
	if r.LimitBuckets {
		i := slices.IndexFunc(r.Buckets, func(g SpaceBucketGrant) bool { return g.Id == bucketId })
		if i < 0 {
			return false, false
		}
		readOnly = readOnly || r.Buckets[i].ReadOnly
	}
	return true, readOnly
}

// ValidSecretPrefix reports whether p can restrict secret references: it
// names its source's scheme, such as op://, and something after it.
func ValidSecretPrefix(p string) bool {
	scheme, rest, ok := strings.Cut(strings.TrimSpace(p), "://")
	return ok && scheme != "" && !strings.ContainsAny(scheme, "/ ") && rest != ""
}

// SecretAllowed reports whether a restricted space may read the secret
// reference: not when secrets are switched off or it's under a denied
// prefix, and, with an allowlist, only under an allowed one. Prefixes match
// without regard to case, as 1Password names don't.
func (r *SpaceRestrictions) SecretAllowed(reference string) bool {
	if r == nil || !r.Active {
		return true
	}
	if r.NoSecrets {
		return false
	}
	if hasSecretPrefix(reference, r.SecretsDeny) {
		return false
	}
	return !r.LimitSecrets || hasSecretPrefix(reference, r.SecretsAllow)
}

func hasSecretPrefix(reference string, prefixes []string) bool {
	reference = strings.ToLower(reference)
	for _, p := range prefixes {
		if strings.HasPrefix(reference, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// narrowSecretsAllow keeps what's allowed by both lists: for each pair of
// prefixes where one contains the other, the narrower one.
func narrowSecretsAllow(a, b []string) []string {
	out := []string{}
	add := func(p string) {
		if !slices.ContainsFunc(out, func(o string) bool { return strings.EqualFold(o, p) }) {
			out = append(out, p)
		}
	}
	for _, x := range a {
		for _, y := range b {
			lx, ly := strings.ToLower(x), strings.ToLower(y)
			switch {
			case strings.HasPrefix(lx, ly):
				add(x)
			case strings.HasPrefix(ly, lx):
				add(y)
			}
		}
	}
	return out
}

// Narrow returns the restrictions with more applied on top. The result is
// never less strict than r in any way: secrets stay blocked once blocked,
// the file mode only gets stricter, and a bucket allowlist only shrinks —
// a bucket missing from either side is dropped, and one read-only on either
// side stays read-only. Denied secret prefixes add up, and an allowlist of
// them only narrows: what's left is what both allow.
func (r SpaceRestrictions) Narrow(more SpaceRestrictions) SpaceRestrictions {
	out := SpaceRestrictions{
		Active:    true,
		NoSecrets: r.NoSecrets || more.NoSecrets,
		Files:     r.Files,
		Locked:    r.Locked || more.Locked,
	}
	if filesRank[more.Files] > filesRank[out.Files] {
		out.Files = more.Files
	}

	switch {
	case r.LimitBuckets && more.LimitBuckets:
		out.LimitBuckets = true
		out.Buckets = []SpaceBucketGrant{}
		for _, g := range r.Buckets {
			i := slices.IndexFunc(more.Buckets, func(m SpaceBucketGrant) bool { return m.Id == g.Id })
			if i >= 0 {
				g.ReadOnly = g.ReadOnly || more.Buckets[i].ReadOnly
				out.Buckets = append(out.Buckets, g)
			}
		}
	case r.LimitBuckets:
		out.LimitBuckets, out.Buckets = true, slices.Clone(r.Buckets)
	case more.LimitBuckets:
		out.LimitBuckets, out.Buckets = true, slices.Clone(more.Buckets)
	}
	if out.LimitBuckets && out.Buckets == nil {
		out.Buckets = []SpaceBucketGrant{}
	}

	out.SecretsDeny = slices.Clone(r.SecretsDeny)
	for _, p := range more.SecretsDeny {
		if !slices.ContainsFunc(out.SecretsDeny, func(o string) bool { return strings.EqualFold(o, p) }) {
			out.SecretsDeny = append(out.SecretsDeny, p)
		}
	}
	switch {
	case r.LimitSecrets && more.LimitSecrets:
		out.LimitSecrets, out.SecretsAllow = true, narrowSecretsAllow(r.SecretsAllow, more.SecretsAllow)
	case r.LimitSecrets:
		out.LimitSecrets, out.SecretsAllow = true, slices.Clone(r.SecretsAllow)
	case more.LimitSecrets:
		out.LimitSecrets, out.SecretsAllow = true, slices.Clone(more.SecretsAllow)
	}
	if out.LimitSecrets && out.SecretsAllow == nil {
		out.SecretsAllow = []string{}
	}
	return out
}
