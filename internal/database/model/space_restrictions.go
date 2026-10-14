package model

import "slices"

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
// space, and are cleared when the space next starts.
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
	// NoCli locks the space's processes out of knot: the knot command and
	// scripts in the space may then only restart or shut it down and show
	// its restrictions. Requests from outside the space, which the agent
	// carries out, such as port forwards and scripts run from the desktop,
	// still work.
	NoCli bool `json:"no_cli" msgpack:"no_cli"`
	// LimitBuckets makes Buckets an allowlist: buckets not in it are hidden.
	LimitBuckets bool               `json:"limit_buckets" msgpack:"limit_buckets"`
	Buckets      []SpaceBucketGrant `json:"buckets" msgpack:"buckets"`
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

// Narrow returns the restrictions with more applied on top. The result is
// never less strict than r in any way: secrets stay blocked once blocked,
// the file mode only gets stricter, and a bucket allowlist only shrinks —
// a bucket missing from either side is dropped, and one read-only on either
// side stays read-only.
func (r SpaceRestrictions) Narrow(more SpaceRestrictions) SpaceRestrictions {
	out := SpaceRestrictions{
		Active:    true,
		NoSecrets: r.NoSecrets || more.NoSecrets,
		Files:     r.Files,
		NoCli:     r.NoCli || more.NoCli,
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
	return out
}
