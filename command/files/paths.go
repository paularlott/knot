package command_files

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// location is one path argument: a local path, a bucket path written
// bucket:key, or - for stdin or stdout.
type location struct {
	raw    string
	stdio  bool
	remote bool
	bucket string // remote only
	key    string // remote only: may be empty (the bucket's root), end in / (a folder) or hold a glob
}

// bucketRef is what may stand before the colon of a bucket path: a bucket's
// short or full name, so letters, digits, dots and hyphens.
var bucketRef = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.-]*[A-Za-z0-9]$`)

// parseLocation decides whether an argument names a bucket path or a local
// one, by its syntax alone: bucket:key when the text before the first colon
// is a bucket name of at least three characters with no path separator
// before it. Anything else is local, so a Windows drive (C:\dir) and ./a:b
// stay local, and a local name with a colon is written with a leading ./
func parseLocation(arg string) (location, error) {
	if arg == "-" {
		return location{raw: arg, stdio: true}, nil
	}
	if i := strings.IndexByte(arg, ':'); i >= 3 && len(arg[:i]) <= 63 && bucketRef.MatchString(arg[:i]) {
		key := strings.TrimPrefix(arg[i+1:], "/")
		if err := checkKey(key); err != nil {
			return location{}, fmt.Errorf("invalid path %q: %w", arg, err)
		}
		return location{raw: arg, remote: true, bucket: arg[:i], key: key}, nil
	}
	if arg == "" {
		return location{}, fmt.Errorf("empty path")
	}
	return location{raw: arg}, nil
}

// checkKey refuses what a URL would silently rewrite to another key: "." and
// ".." segments and empty ones. A trailing "/" naming a folder is fine.
func checkKey(key string) error {
	if key == "" {
		return nil
	}
	parts := strings.Split(key, "/")
	for i, part := range parts {
		if part == "." || part == ".." || (part == "" && i != len(parts)-1) {
			return fmt.Errorf("no empty, . or .. segments")
		}
	}
	return nil
}

// String writes a location as the user would.
func (l location) String() string {
	switch {
	case l.stdio:
		return "-"
	case l.remote:
		return l.bucket + ":" + l.key
	}
	return l.raw
}

// remoteName writes a bucket path.
func remoteName(bucket, key string) string { return bucket + ":" + key }

// isFolderKey reports whether a key names a folder: the bucket's root or
// anything ending in /.
func isFolderKey(key string) bool { return key == "" || strings.HasSuffix(key, "/") }

// hasGlob reports whether a path holds a wildcard. To name a file that has
// one of these characters in its name, put it in brackets: a[*]b.
func hasGlob(s string) bool { return strings.ContainsAny(s, "*?[") }

// globRoot splits a pattern into the folder before its first wildcard
// segment, with its trailing /, and the pattern's segments.
func globRoot(pattern string) (string, []string) {
	segs := strings.Split(pattern, "/")
	var lit []string
	for _, s := range segs {
		if hasGlob(s) {
			break
		}
		lit = append(lit, s)
	}
	root := strings.Join(lit, "/")
	if root != "" {
		root += "/"
	}
	// A pattern with no wildcard has no root: it is the whole path.
	if len(lit) == len(segs) {
		return "", segs
	}
	return root, segs
}

// matchSegs matches path segments against pattern segments: * and ? and
// [...] within a segment as in a shell, and a segment of ** for any number of
// whole segments.
func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], segs[0]); err != nil || !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// matchGlob reports whether a slash separated key matches a pattern and, if
// so, its path below the pattern's root, which is where it lands in a
// destination folder. With folders set a key also matches when a leading
// part of it, a folder, matches, so everything below a matched folder is
// taken.
func matchGlob(pattern, key string, folders bool) (string, bool) {
	root, pat := globRoot(pattern)
	if !strings.HasPrefix(key, root) {
		return "", false
	}
	segs := strings.Split(key, "/")
	if matchSegs(pat, segs) {
		return strings.TrimPrefix(key, root), true
	}
	if folders {
		for n := 1; n < len(segs); n++ {
			if matchSegs(pat, segs[:n]) {
				return strings.TrimPrefix(key, root), true
			}
		}
	}
	return "", false
}

// globFolder returns the folder, ending in /, that a key sits below when a
// leading part of the key matches the pattern.
func globFolder(pattern, key string) (string, bool) {
	root, pat := globRoot(pattern)
	if !strings.HasPrefix(key, root) {
		return "", false
	}
	segs := strings.Split(key, "/")
	for n := 1; n < len(segs); n++ {
		if matchSegs(pat, segs[:n]) {
			return strings.Join(segs[:n], "/") + "/", true
		}
	}
	return "", false
}

// requireRemote parses an argument that must be a bucket path, with a
// pointer for the old way of writing one.
func requireRemote(arg string) (location, error) {
	loc, err := parseLocation(arg)
	if err != nil {
		return loc, err
	}
	if !loc.remote {
		if bucketRef.MatchString(arg) {
			return loc, fmt.Errorf("%q is not a bucket path: write %s: for the bucket itself, or %s:path for a file or folder in it", arg, arg, arg)
		}
		return loc, fmt.Errorf("%q is not a bucket path: write bucket:path", arg)
	}
	return loc, nil
}
