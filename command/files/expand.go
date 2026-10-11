package command_files

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/paularlott/knot/apiclient"
)

// source is one file to act on, found by expanding a path argument.
type source struct {
	local  string // a local file's path
	bucket string // a bucket file
	key    string
	rel    string // where it goes below a destination folder, slash separated
	size   int64
	marker bool // a folder marker object (key ends in /): it makes a folder, not a file
}

func (s source) String() string {
	if s.local != "" {
		return s.local
	}
	return remoteName(s.bucket, s.key)
}

// remoteInfo says what a bucket path names.
type remoteInfo struct {
	file   *apiclient.FileObjectInfo // the file with exactly this key, if any
	folder bool                      // files below key + "/" exist (or the key is a folder, which may be empty)
}

// statRemote finds out whether a bucket path is a file, a folder, or both.
// It also proves the bucket exists and can be read.
func statRemote(ctx context.Context, client *apiclient.ApiClient, bucket, key string) (remoteInfo, error) {
	var info remoteInfo
	if isFolderKey(key) {
		// A bare bucket or a key ending in /; look to see it is readable and,
		// for a folder, that it holds something.
		list, err := client.ListFileObjects(ctx, bucket, key, "", "", 1)
		if err != nil {
			return info, apiError(err)
		}
		info.folder = key == "" || len(list.Objects) > 0
		return info, nil
	}

	// The key itself sorts before everything else that starts with it.
	list, err := client.ListFileObjects(ctx, bucket, key, "", "", 1)
	if err != nil {
		return info, apiError(err)
	}
	if len(list.Objects) > 0 && list.Objects[0].Key == key {
		o := list.Objects[0]
		info.file = &o
	}
	list, err = client.ListFileObjects(ctx, bucket, key+"/", "", "", 1)
	if err != nil {
		return info, apiError(err)
	}
	info.folder = len(list.Objects) > 0
	return info, nil
}

// remoteSources expands one bucket path argument into the files it names. A
// wildcard matches files, and with recursive the folders it matches too,
// taking everything below them. A folder, or a bucket, needs recursive.
// folder reports whether the result can only go into a destination folder.
// The marker object that makes a folder exist when it is empty is left out
// unless ownMarker, as a delete must take it too.
func remoteSources(ctx context.Context, client *apiclient.ApiClient, loc location, recursive, ownMarker bool) (list []source, folder bool, err error) {
	bucket, key := loc.bucket, loc.key

	if hasGlob(key) {
		root, _ := globRoot(key)
		objects, _, err := listAll(ctx, client, bucket, root, "")
		if err != nil {
			return nil, true, err
		}
		for _, o := range objects {
			if rel, ok := matchGlob(key, o.Key, recursive); ok && rel != "" {
				list = append(list, source{bucket: bucket, key: o.Key, rel: rel, size: o.Size, marker: strings.HasSuffix(o.Key, "/")})
			}
		}
		if len(list) == 0 {
			return nil, true, fmt.Errorf("no files match %s", loc)
		}
		return list, true, nil
	}

	info, err := statRemote(ctx, client, bucket, key)
	if err != nil {
		return nil, false, err
	}

	// A folder is taken in preference to a file of the same name when
	// recursing; without it the file is the only thing that can be meant.
	if info.folder && (recursive || info.file == nil) {
		if !recursive {
			return nil, true, fmt.Errorf("%s is a folder, use -r to copy it", loc)
		}
		prefix := dirPrefix(key)
		objects, _, err := listAll(ctx, client, bucket, prefix, "")
		if err != nil {
			return nil, true, err
		}
		for _, o := range objects {
			rel := strings.TrimPrefix(o.Key, prefix)
			if rel == "" && !ownMarker {
				continue // the folder's own marker
			}
			list = append(list, source{bucket: bucket, key: o.Key, rel: rel, size: o.Size, marker: strings.HasSuffix(o.Key, "/")})
		}
		return list, true, nil
	}
	if info.file == nil {
		return nil, false, fmt.Errorf("%s: no such file or folder", loc)
	}
	return []source{{bucket: bucket, key: key, rel: path.Base(key), size: info.file.Size}}, false, nil
}

// localSources expands one local path argument into the files it names, the
// same way. A wildcard is expanded here only when no file has that literal
// name, which a shell has normally done already.
func localSources(arg string, recursive bool) (list []source, folder bool, err error) {
	if _, statErr := os.Stat(arg); statErr != nil && hasGlob(arg) {
		return localGlob(arg, recursive)
	}

	info, err := os.Stat(arg)
	if err != nil {
		return nil, false, err
	}
	if info.Mode().IsRegular() {
		return []source{{local: arg, rel: filepath.Base(arg), size: info.Size()}}, false, nil
	}
	if !info.IsDir() {
		return nil, false, fmt.Errorf("%s is not a regular file", arg)
	}
	if !recursive {
		return nil, true, fmt.Errorf("%s is a directory, use -r to copy it", arg)
	}

	// Walk the real directory; a symlinked root would look empty.
	root, err := filepath.EvalSymlinks(arg)
	if err != nil {
		return nil, true, err
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !d.Type().IsRegular() {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		list = append(list, source{local: p, rel: filepath.ToSlash(rel), size: fi.Size()})
		return nil
	})
	return list, true, err
}

// localGlob expands a pattern against the local files.
func localGlob(pattern string, recursive bool) ([]source, bool, error) {
	slashed := filepath.ToSlash(pattern)
	root, segs := globRoot(slashed)
	start := filepath.FromSlash(strings.TrimSuffix(root, "/"))
	if root == "" {
		start = "."
	} else if start == "" {
		start = string(filepath.Separator) // a pattern anchored at /
	}
	deep := recursive || strings.Contains("/"+slashed+"/", "/**/")

	var list []source
	err := filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		key := filepath.ToSlash(p)
		if root == "" {
			key = strings.TrimPrefix(key, "./")
		}
		if d.IsDir() {
			// Without ** or -r nothing deeper than the pattern can match.
			if p != start && !deep && len(strings.Split(key, "/")) >= len(segs) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if rel, ok := matchGlob(slashed, key, recursive); ok && rel != "" {
			fi, err := d.Info()
			if err != nil {
				return err
			}
			list = append(list, source{local: p, rel: rel, size: fi.Size()})
		}
		return nil
	})
	if err != nil {
		return nil, true, err
	}
	if len(list) == 0 {
		return nil, true, fmt.Errorf("no files match %s", pattern)
	}
	return list, true, nil
}
