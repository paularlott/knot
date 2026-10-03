package filestore

import (
	"errors"
	"regexp"
	"strings"
)

// Buckets are namespaced by their owner: the full name is
// "<username>--<name>", the username lowercased. Usernames and the short
// names users choose never contain "--", so the first "--" always separates
// the owner from the short name. A reference without "--" is the caller's
// own bucket.

// NameSeparator joins the owner's username and the short bucket name.
const NameSeparator = "--"

// MaxShortNameLen is the longest short bucket name; with usernames of at most
// 30 characters the full name stays within S3's 63.
const MaxShortNameLen = 30

var (
	ErrInvalidShortName = errors.New("invalid bucket name: use 3-30 lowercase letters, digits and hyphens, starting and ending with a letter or digit, without --")
	ErrNameTooLong      = errors.New("bucket name too long: your username and the bucket name together must be at most 61 characters")
	ErrBucketNamespace  = errors.New("bucket names are created under your own username")
	ErrUsernameUnusable = errors.New("your username cannot be used in a bucket name; ask an administrator to transfer a bucket to you instead")
)

var shortNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)

// ValidShortName reports whether name is a valid short bucket name.
func ValidShortName(name string) bool {
	return len(name) >= 3 && len(name) <= MaxShortNameLen && shortNameRe.MatchString(name) && !strings.Contains(name, NameSeparator)
}

// ownerPrefix is the namespace of a username.
func ownerPrefix(username string) string {
	return strings.ToLower(username)
}

// splitName returns the owner prefix and short name of a full bucket name;
// ok is false for a name without a namespace.
func splitName(name string) (prefix, short string, ok bool) {
	return strings.Cut(name, NameSeparator)
}

// ShortName returns the short part of a bucket name.
func ShortName(name string) string {
	if _, short, ok := splitName(name); ok {
		return short
	}
	return name
}

// FullName builds the full name of a bucket called short owned by username.
func FullName(username, short string) (string, error) {
	if !ValidShortName(short) {
		return "", ErrInvalidShortName
	}
	full := ownerPrefix(username) + NameSeparator + short
	if len(full) > 63 {
		return "", ErrNameTooLong
	}
	if !ValidBucketName(full) {
		// A username from before the stricter rules, e.g. ending in "-".
		return "", ErrUsernameUnusable
	}
	return full, nil
}

// NewBucketName turns the name a user asked for into the full name of a new
// bucket they will own: a short name gets their prefix, and a full name must
// already carry it.
func NewBucketName(p *Principal, name string) (string, error) {
	if prefix, short, ok := splitName(name); ok {
		if prefix != ownerPrefix(p.Username) {
			return "", ErrBucketNamespace
		}
		name = short
	}
	return FullName(p.Username, name)
}

// ResolveName turns a bucket reference into a full name: a short name is the
// caller's own bucket.
func ResolveName(p *Principal, name string) string {
	if p == nil || p.Username == "" || strings.Contains(name, NameSeparator) {
		return name
	}
	return ownerPrefix(p.Username) + NameSeparator + name
}

// DisplayName is how a bucket is shown to p: the short name for their own
// buckets, the full name for everyone else's.
func DisplayName(p *Principal, name string) string {
	if p != nil && p.Username != "" {
		if prefix, short, ok := splitName(name); ok && prefix == ownerPrefix(p.Username) {
			return short
		}
	}
	return name
}
