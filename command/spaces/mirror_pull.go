package command_spaces

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/command/cmdutil"
	"github.com/paularlott/knot/internal/util/writedir"
)

// isSpacePath reports whether a mirror argument names a space, as
// <space>:<path>. A one-letter name is a Windows drive letter, and a path
// separator before the colon makes it a local path that holds a colon.
func isSpacePath(arg string) bool {
	colon := strings.Index(arg, ":")
	return colon > 1 && !strings.ContainsAny(arg[:colon], `/\`)
}

// localEntry is what pull knows about a path under the local folder.
type localEntry struct {
	isDir      bool
	isLink     bool
	linkTarget string
	size       int64
	mtime      time.Time
	perm       uint32
}

// download is the per-file work item for pull.
type download struct {
	rel           string
	symlinkTarget string // when non-empty, create symlink instead of fetching content
	size          int64
}

// tempPrefix marks a download in progress, so a later run ignores what an
// interrupted one left behind and clears it away.
const tempPrefix = ".knot-mirror-"

// runPull executes one mirror cycle from the space: fetch the remote
// path+metadata map, walk local, skip files that already match, download
// the rest in parallel, then delete any local path the space doesn't have.
// Failures of individual files don't abort the run.
func (o *mirrorOptions) runPull(ctx context.Context) (*mirrorStats, error) {
	stats := &mirrorStats{}
	o.resolveRoot()

	remoteSet, err := o.fetchRemoteSet(ctx)
	if err != nil {
		if o.hash && strings.Contains(err.Error(), "503") {
			return stats, fmt.Errorf("list remote: %w\n\nThe agent timed out hashing the remote tree (common for large spaces with --hash or --verify).\nTry without --hash — the default mtime+size comparison is fast and sufficient for most cases", err)
		}
		return stats, fmt.Errorf("list remote: %w", err)
	}
	excludes := compileExcludes(o.excludes)
	for rel, meta := range remoteSet {
		// A large upload a push left unfinished is not a file to fetch.
		if excludes(rel, meta.isDir) || (!meta.isDir && isUploadTemp(rel)) {
			delete(remoteSet, rel)
		}
	}

	if o.verify {
		return o.runVerify(ctx, remoteSet, stats)
	}

	localSet, err := o.walkLocal(excludes)
	if err != nil {
		return stats, err
	}
	// Files named as an unfinished upload are left alone here too: the
	// space's are not fetched, so one here would otherwise look like a file
	// the space doesn't have, and be deleted.
	for rel, l := range localSet {
		if !l.isDir && isUploadTemp(rel) {
			delete(localSet, rel)
		}
	}

	// Refuse to wipe the local folder when the space has nothing to mirror,
	// as a mistyped path or swapped arguments would.
	if len(remoteSet) == 0 && len(localSet) > 0 {
		return stats, fmt.Errorf("%s:%s is empty or missing, so mirroring it would delete everything in %s; check the path, or use --dry-run to see what would change", o.spaceName, o.remoteDir, o.localRoot)
	}

	// Classify each remote entry: skip, download, or (for a directory where
	// a local file stands) clear the way.
	var downloads []download
	var conflicts []string
	var dirs []localDir
	toHash := map[string]string{}
	for rel, r := range remoteSet {
		l, ok := localSet[rel]
		if !r.isDir && r.linkTarget == "" && ok && !l.isDir && !l.isLink && l.size == r.size && o.hash && r.hash != 0 {
			toHash[rel] = filepath.Join(o.localRoot, filepath.FromSlash(rel))
		}
	}
	hashes := hashFilesParallel(toHash, o.parallel)

	for rel, r := range remoteSet {
		l, hadLocal := localSet[rel]
		delete(localSet, rel)

		if r.isDir {
			if hadLocal && !l.isDir {
				conflicts = append(conflicts, rel)
			}
			dirs = append(dirs, localDir{rel: rel, perm: r.perm})
			continue
		}
		stats.scanned.Add(1)

		if r.linkTarget != "" {
			if hadLocal && l.isLink && l.linkTarget == r.linkTarget {
				stats.skipped.Add(1)
				continue
			}
			downloads = append(downloads, download{rel: rel, symlinkTarget: r.linkTarget})
			continue
		}

		if hadLocal && !l.isDir && !l.isLink && l.size == r.size {
			if o.hash && r.hash != 0 {
				if hashes[rel] == r.hash {
					stats.hashSkipped.Add(1)
					stats.skipped.Add(1)
					o.fixLocalMode(rel, l.perm, r.perm, stats)
					continue
				}
			} else if mtimeWithin(l.mtime, r.mtime, mtimeTolerance) {
				stats.mtimeSkipped.Add(1)
				stats.skipped.Add(1)
				o.fixLocalMode(rel, l.perm, r.perm, stats)
				continue
			}
		}
		if o.debug {
			fmt.Fprintf(os.Stderr, "  ? %s — %s\n", rel, downloadReason(hadLocal, l, r))
		}
		downloads = append(downloads, download{rel: rel, size: r.size})
	}

	// A local file where the space has a directory goes first, so the
	// directory's files have somewhere to land.
	o.deleteLocal(conflicts, stats)

	// Read-only folders here are opened while files are added to or
	// removed from them (see writedir), then given back their modes.
	o.downloadInParallel(ctx, downloads, stats)

	// Then the folders the downloads didn't make (empty ones), and folder
	// modes.
	o.makeLocalDirs(ctx, dirs, stats)

	// Anything left in localSet isn't in the space → remove it. A removed
	// directory takes its contents with it, so they're skipped.
	extras := make([]string, 0, len(localSet))
	for rel := range localSet {
		extras = append(extras, rel)
	}
	o.deleteLocal(withoutNested(extras), stats)

	return stats, nil
}

// fixLocalMode gives a file that needs no download the space's permission
// bits, when only they changed (chmod keeps size and mtime).
func (o *mirrorOptions) fixLocalMode(rel string, local, remote uint32, stats *mirrorStats) {
	if remote == 0 || local == remote || runtime.GOOS == "windows" {
		return
	}
	if !o.dryRun {
		if err := os.Chmod(filepath.Join(o.localRoot, filepath.FromSlash(rel)), os.FileMode(remote)); err != nil {
			stats.failed.Add(1)
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			return
		}
	}
	if o.verbose {
		fmt.Fprintf(os.Stderr, "  ↓ %s (mode %o)\n", rel, remote)
	}
}

// makeLocalDirs makes the space's folders here with their modes, those
// deepest first, so a folder is never made read-only before the ones in it.
func (o *mirrorOptions) makeLocalDirs(ctx context.Context, dirs []localDir, stats *mirrorStats) {
	sortDirs(dirs, true)
	for _, d := range dirs {
		if ctx.Err() != nil {
			return
		}
		dest := filepath.Join(o.localRoot, filepath.FromSlash(d.rel))
		info, err := os.Stat(dest)
		missing := err != nil
		setMode := d.perm != 0 && runtime.GOOS != "windows" && (missing || uint32(info.Mode().Perm()) != d.perm)
		if !missing && !setMode {
			continue
		}
		if o.dryRun {
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  ↓ %s/ (dry-run)\n", d.rel)
			}
			stats.uploaded.Add(1)
			continue
		}
		if missing {
			err = writedir.MkdirAll(dest, 0755)
		}
		if err == nil && setMode {
			err = os.Chmod(dest, os.FileMode(d.perm))
		}
		if err != nil {
			stats.failed.Add(1)
			fmt.Fprintf(os.Stderr, "  ! %s/: %v\n", d.rel, err)
			continue
		}
		stats.uploaded.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  ↓ %s/\n", d.rel)
		}
	}
}

// walkLocal lists the local folder, keyed by slash-relative path, leaving
// out excluded paths and downloads an earlier run didn't finish (which it
// removes). A missing folder is empty.
func (o *mirrorOptions) walkLocal(excludes func(string, bool) bool) (map[string]localEntry, error) {
	set := map[string]localEntry{}
	err := filepath.WalkDir(o.localRoot, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if p == o.localRoot && os.IsNotExist(err) {
				return filepath.SkipAll
			}
			return nil // skip unreadable entries
		}
		if p == o.localRoot {
			return nil
		}
		rel, err := filepath.Rel(o.localRoot, p)
		if err != nil {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if excludes(relSlash, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), tempPrefix) {
			if !o.dryRun {
				os.Remove(p)
			}
			return nil
		}
		switch {
		case d.IsDir():
			set[relSlash] = localEntry{isDir: true}
		case d.Type()&os.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			set[relSlash] = localEntry{isLink: true, linkTarget: target}
		default:
			info, err := d.Info()
			if err != nil {
				return nil
			}
			set[relSlash] = localEntry{size: info.Size(), mtime: info.ModTime(), perm: uint32(info.Mode().Perm())}
		}
		return nil
	})
	return set, err
}

// downloadReason explains, for --debug, why a file is being fetched.
func downloadReason(hadLocal bool, l localEntry, r remoteMeta) string {
	switch {
	case !hadLocal:
		return "new (not on local)"
	case l.isDir:
		return "local is a directory"
	case l.isLink:
		return "local is a symlink"
	case l.size != r.size:
		return fmt.Sprintf("size differs: local=%d remote=%d", l.size, r.size)
	case r.hash != 0:
		return fmt.Sprintf("hash mismatch (remote crc64=%d) — content differs despite same size", r.hash)
	}
	return fmt.Sprintf("mtime drift: local=%s remote=%s (tol=%s)",
		l.mtime.UTC().Format(time.RFC3339Nano), r.mtime.UTC().Format(time.RFC3339Nano), mtimeTolerance)
}

// downloadInParallel runs N workers that drain the download queue, one file
// per worker at a time. Failures don't abort the cycle.
func (o *mirrorOptions) downloadInParallel(ctx context.Context, downloads []download, stats *mirrorStats) {
	jobs := make(chan download)
	var wg sync.WaitGroup
	for i := 0; i < o.parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for d := range jobs {
				if ctx.Err() != nil {
					return
				}
				if o.dryRun {
					if o.verbose {
						fmt.Fprintf(os.Stderr, "  ↓ %s (dry-run)\n", d.rel)
					}
					stats.uploaded.Add(1)
					continue
				}
				n, err := o.downloadOne(ctx, d)
				if err != nil {
					stats.failed.Add(1)
					fmt.Fprintf(os.Stderr, "  ! %s: %v\n", d.rel, cmdutil.Describe(err))
					continue
				}
				stats.uploaded.Add(1)
				stats.bytes.Add(n)
				if o.verbose {
					fmt.Fprintf(os.Stderr, "  ↓ %s\n", d.rel)
				}
			}
		}()
	}
	for _, d := range downloads {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- d:
		}
	}
	close(jobs)
	wg.Wait()
}

// downloadOne fetches a file from the space in chunkSize pieces into a
// temporary file beside its destination, sets its mtime and permission
// bits, then renames it into place, so an interrupted download never
// leaves a partial file under the real name. Returns the bytes fetched.
func (o *mirrorOptions) downloadOne(ctx context.Context, d download) (int64, error) {
	dest := filepath.Join(o.localRoot, filepath.FromSlash(d.rel))
	dir := filepath.Dir(dest)
	if err := writedir.MkdirAll(dir, 0755); err != nil {
		return 0, err
	}

	if d.symlinkTarget != "" {
		if err := clearLocal(dest); err != nil {
			return 0, err
		}
		// A file here is replaced too: a symlink can't be renamed over it.
		return 0, writedir.In(func() error {
			if info, err := os.Lstat(dest); err == nil && !info.IsDir() {
				os.Remove(dest)
			}
			return os.Symlink(d.symlinkTarget, dest)
		}, dir)
	}

	var tmp *os.File
	err := writedir.In(func() (err error) {
		tmp, err = os.CreateTemp(dir, tempPrefix+"*")
		return err
	}, dir)
	if err != nil {
		return 0, err
	}
	done := false
	defer func() {
		if !done {
			tmp.Close()
			writedir.Remove(tmp.Name())
		}
	}()

	src := path.Join(o.remoteDir, d.rel)
	var offset, size, mtimeNs int64 = 0, -1, 0
	var perm uint32
	for size < 0 || offset < size {
		part, err := o.client.ReadSpaceFileBytes(ctx, o.spaceID, src, offset, chunkSize)
		if err != nil {
			return offset, err
		}
		if size < 0 {
			size, mtimeNs, perm = part.FileSize, part.MtimeNs, part.FilePerm
		}
		if len(part.Content) == 0 {
			if offset < size {
				return offset, fmt.Errorf("the file changed in the space while it was downloading")
			}
			break
		}
		if _, err := tmp.Write(part.Content); err != nil {
			return offset, err
		}
		offset += int64(len(part.Content))
	}

	if err := tmp.Close(); err != nil {
		return offset, err
	}
	if perm != 0 {
		os.Chmod(tmp.Name(), os.FileMode(perm))
	}
	if mtimeNs != 0 {
		mtime := time.Unix(0, mtimeNs)
		os.Chtimes(tmp.Name(), mtime, mtime)
	}
	if err := clearLocal(dest); err != nil {
		return offset, err
	}
	if err := writedir.Rename(tmp.Name(), dest); err != nil {
		return offset, err
	}
	done = true
	return offset, nil
}

// clearLocal removes whatever is at p unless it's a regular file, which a
// rename replaces in one step.
func clearLocal(p string) error {
	info, err := os.Lstat(p)
	if err != nil || info.Mode().IsRegular() {
		return nil
	}
	return writedir.RemoveAll(p)
}

// deleteLocal removes the given slash-relative paths under the local
// folder, directories with everything in them.
func (o *mirrorOptions) deleteLocal(rels []string, stats *mirrorStats) {
	for _, rel := range rels {
		if o.dryRun {
			if o.verbose {
				fmt.Fprintf(os.Stderr, "  − %s (dry-run)\n", rel)
			}
			stats.deleted.Add(1)
			continue
		}
		if err := writedir.RemoveAll(filepath.Join(o.localRoot, filepath.FromSlash(rel))); err != nil {
			stats.failed.Add(1)
			fmt.Fprintf(os.Stderr, "  ! %s: %v\n", rel, err)
			continue
		}
		stats.deleted.Add(1)
		if o.verbose {
			fmt.Fprintf(os.Stderr, "  − %s\n", rel)
		}
	}
}

// withoutNested drops the paths that lie inside another path in the list,
// which removing that path already covers.
func withoutNested(rels []string) []string {
	set := make(map[string]bool, len(rels))
	for _, rel := range rels {
		set[rel] = true
	}
	out := make([]string, 0, len(rels))
	for _, rel := range rels {
		nested := false
		for parent := path.Dir(rel); parent != "." && parent != "/"; parent = path.Dir(parent) {
			if set[parent] {
				nested = true
				break
			}
		}
		if !nested {
			out = append(out, rel)
		}
	}
	return out
}
