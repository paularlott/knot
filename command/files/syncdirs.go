package command_files

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/command/cmdutil"
)

// Folders are synced as the NFS server keeps them: a marker, an empty object
// whose key is the folder's path ending in /, holds the folder's mode, so a
// folder is kept when it is empty, and with its mode. Every folder synced up
// gets a marker, as every folder made over NFS has one, so a tree looks the
// same in the bucket however it got there. A folder with no marker, written
// by an S3 client or before knot kept folders, is implied by the files below
// it and has no mode of its own.
//
// Folders are planned with the files, under identities ending in /, and
// made, changed and removed after the files are: a folder's mode is set once
// nothing more is written into it, and a folder is removed once the files in
// it are.

type localDir struct {
	rel   string // slash-separated, below the directory, ending in /
	mtime time.Time
	mode  os.FileMode // permission bits, 0 where the platform has none
}

// isDirId reports whether an identity is a folder's.
func isDirId(id string) bool { return strings.HasSuffix(id, "/") }

func newLocalDir(rel string, info os.FileInfo) *localDir {
	return &localDir{rel: rel, mtime: info.ModTime(), mode: apiclient.LocalFileAttrs(info).Mode}
}

// dirExcluded reports whether a folder, rel without its trailing /, is left
// out of the sync, itself or as part of a folder that is.
func (s *syncer) dirExcluded(rel string) bool {
	for d := rel; d != "." && d != "" && d != "/"; d = path.Dir(d) {
		if s.excluded(d, true) {
			return true
		}
	}
	return false
}

// addMarker adds a bucket folder's marker to markers, unless the folder is
// the synced folder itself, excluded, or would land outside the directory.
func (s *syncer) addMarker(markers map[string]*remoteFile, f *remoteFile) {
	rel := strings.TrimSuffix(f.rel, "/")
	if rel == "" || s.dirExcluded(rel) {
		return
	}
	if _, err := within(s.opt.dir, rel); err != nil {
		s.warnOnce("outside:"+f.rel, "skipped: the name would land outside the directory", "folder", remoteName(s.opt.bucket, f.key))
		return
	}
	markers[s.identity(f.rel)] = f
}

// statLocalDir looks at one folder afresh, rel ending in /, nil when there is
// no folder to sync there.
func (s *syncer) statLocalDir(rel string) *localDir {
	name := strings.TrimSuffix(rel, "/")
	if name == "" || s.dirExcluded(name) || linkOnPath(s.opt.dir, name) != "" {
		return nil
	}
	info, err := os.Lstat(filepath.Join(s.opt.dir, filepath.FromSlash(name)))
	if err != nil || !info.IsDir() {
		return nil
	}
	if s.fold {
		if real := s.spelling(name); real != "" {
			name = real
		}
	}
	return newLocalDir(name+"/", info)
}

// remoteFolders is every folder the bucket holds something in or below, by
// identity: folders with markers and the folders of every file.
func (s *syncer) remoteFolders() map[string]bool {
	below := make(map[string]bool)
	add := func(rel string) {
		for d := path.Dir(strings.TrimSuffix(rel, "/")); d != "." && d != "/" && d != ""; d = path.Dir(d) {
			id := s.identity(d + "/")
			if below[id] {
				return
			}
			below[id] = true
		}
	}
	for _, f := range s.remote {
		add(f.rel)
	}
	for id, m := range s.markers {
		below[id] = true
		add(m.rel)
	}
	return below
}

// planDirs decides what each folder needs.
func (s *syncer) planDirs(ids []string) []action {
	var below map[string]bool
	if s.opt.mode == modeDown && s.opt.del {
		below = s.remoteFolders()
	}
	var out []action
	for _, id := range ids {
		d, m, b := s.dirs[id], s.markers[id], s.base[id]
		a := action{id: id, dir: true, d: d, r: m, b: b}
		switch s.opt.mode {
		case modeUp:
			switch {
			case d != nil && m == nil:
				a.kind = actUpload
			case d != nil:
				if d.mode != 0 && d.mode != m.mode {
					a.kind, a.setMode = actAgree, modeToRemote
				}
			case m != nil && s.opt.del:
				a.kind = actDeleteRemote
			}
		case modeDown:
			switch {
			case m != nil && d == nil:
				a.kind = actDownload
			case m != nil:
				if m.mode != 0 && d.mode != 0 && d.mode != m.mode {
					a.kind, a.setMode = actAgree, modeToLocal
				}
			case d != nil && s.opt.del && !below[id]:
				a.kind = actDeleteLocal
			}
		case modeTwoWay:
			switch {
			case d != nil && m != nil:
				a.kind = actAgree
				var base os.FileMode
				if b != nil {
					base = os.FileMode(b.Mode)
				}
				a.setMode = twoWayMode(d.mode, m.mode, base)
			case d != nil:
				// Gone from the bucket since both had it, or new here.
				if b != nil && s.opt.del {
					a.kind = actDeleteLocal
				} else {
					a.kind = actUpload
				}
			case m != nil:
				// Gone from here since both had it, or new in the bucket.
				if b != nil && s.opt.del {
					a.kind = actDeleteRemote
				} else {
					a.kind = actDownload
				}
			case b != nil:
				a.kind = actForget
			}
		}
		if a.kind != actNone {
			out = append(out, a)
		}
	}
	return out
}

// applyDirs carries out the folders' actions once the files' are done:
// removals first, a folder's contents before the folder, then the folders
// made and the modes set, again a folder's contents first, so a folder made
// read-only is so only once everything in it is written. Bucket markers are
// written several at a time; a first sync up of a large tree writes many.
func (s *syncer) applyDirs(ctx context.Context, acts []action) []error {
	sort.Slice(acts, func(i, j int) bool { return acts[i].id > acts[j].id })
	var errs []error
	var remote []action
	local := func(a action) {
		if err := s.applyDir(ctx, a); err != nil {
			errs = append(errs, err)
		}
	}
	for _, a := range acts {
		switch {
		case a.kind == actDeleteLocal:
			local(a)
		case a.kind == actDeleteRemote:
			remote = append(remote, a)
		}
	}
	errs = append(errs, s.runParallel(ctx, remote, transferWorkers)...)
	remote = remote[:0]
	for _, a := range acts {
		switch {
		case a.kind == actDeleteLocal || a.kind == actDeleteRemote:
		case a.kind == actUpload || a.setMode == modeToRemote:
			remote = append(remote, a)
		default:
			local(a)
		}
	}
	return append(errs, s.runParallel(ctx, remote, transferWorkers)...)
}

// applyDir carries out one folder's action. It is called from several
// goroutines.
func (s *syncer) applyDir(ctx context.Context, a action) error {
	switch a.kind {
	case actAgree:
		switch a.setMode {
		case modeToLocal:
			return s.chmodDir(a)
		case modeToRemote:
			return s.writeMarker(ctx, a.id, a.d, a.r)
		}
		s.agreeDir(a.id, a.d, a.r)
	case actForget:
		s.mu.Lock()
		delete(s.base, a.id)
		s.baseDirt = true
		s.mu.Unlock()
	case actUpload:
		return s.writeMarker(ctx, a.id, a.d, nil)
	case actDownload:
		return s.makeDir(a)
	case actDeleteLocal:
		return s.removeDir(ctx, a)
	case actDeleteRemote:
		return s.deleteMarker(ctx, a)
	}
	return nil
}

// agreeDir records that both sides hold a folder.
func (s *syncer) agreeDir(id string, d *localDir, m *remoteFile) {
	if s.opt.mode != modeTwoWay || s.opt.dryRun || d == nil || m == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if b := s.base[id]; b != nil && b.Key == m.key && b.Mode == uint32(d.mode) {
		return
	}
	s.base[id] = &baseEntry{Key: m.key, Mode: uint32(d.mode)}
	s.baseDirt = true
}

// writeMarker writes a folder's marker with the local folder's mode: a new
// one, or have rewritten when only the mode differs.
func (s *syncer) writeMarker(ctx context.Context, id string, d *localDir, have *remoteFile) error {
	key := s.opt.prefix + d.rel
	if have != nil {
		key = have.key // keep the bucket's spelling of the name
	}
	name := remoteName(s.opt.bucket, key)
	if s.opt.dryRun {
		if have != nil {
			s.log.Info("would set mode", "folder", name, "mode", fmt.Sprintf("%04o", d.mode))
			s.count(func(c *passCounts) { c.modes++ })
		} else {
			s.log.Info("would make folder", "folder", name)
			s.count(func(c *passCounts) { c.folders++ })
		}
		return nil
	}
	// The folder may have gone since the pass looked.
	if info, err := os.Lstat(filepath.Join(s.opt.dir, filepath.FromSlash(d.rel))); err != nil || !info.IsDir() {
		return nil
	}
	attrs := apiclient.FileAttrs{Mtime: d.mtime}
	if d.mode != 0 {
		attrs.Mode = os.ModeDir | d.mode
	}
	var info *apiclient.FileObjectInfo
	var err error
	switch {
	case have != nil:
		// Only the mode changes: the marker's other attributes, the inode
		// number the NFS server gave the folder among them, are kept.
		meta := make(map[string]string, len(have.meta)+1)
		for k, v := range have.meta {
			meta[k] = v
		}
		meta[apiclient.FileModeMeta] = apiclient.FormatFileMode(attrs.Mode)
		info, err = s.client.CopyFileObject(ctx, apiclient.FileCopyRequest{
			SourceBucket: s.opt.bucket, SourceKey: key, DestBucket: s.opt.bucket, DestKey: key,
			IfMatch: have.etag, Meta: meta,
		})
	case s.opt.mode == modeTwoWay:
		info, err = s.client.PutFileObjectIfAbsent(ctx, s.opt.bucket, key, strings.NewReader(""), 0, "", attrs)
	default:
		info, err = s.client.PutFileObject(ctx, s.opt.bucket, key, strings.NewReader(""), 0, "", attrs)
	}
	if err != nil {
		if apiclient.IsPreconditionFailed(err) {
			s.log.Debug("changed in the bucket meanwhile: looking again", "folder", name)
			return nil
		}
		return fmt.Errorf("%s: %w", name, cmdutil.CleanErr(err))
	}
	m := fromInfo(*info, s.opt.prefix)
	s.mu.Lock()
	s.markers[id] = m
	if have != nil {
		s.counts.modes++
	} else {
		s.counts.folders++
	}
	s.mu.Unlock()
	if have != nil {
		s.log.Info("set mode", "folder", name, "mode", fmt.Sprintf("%04o", d.mode))
	} else {
		s.log.Info("made folder", "folder", name)
	}
	s.agreeDir(id, d, m)
	return nil
}

// makeDir makes a bucket folder here, with its marker's mode.
func (s *syncer) makeDir(a action) error {
	m := a.r
	rel := strings.TrimSuffix(m.rel, "/")
	local, err := within(s.opt.dir, rel)
	if err != nil {
		return err
	}
	if s.opt.dryRun {
		s.log.Info("would make folder", "folder", rel)
		s.count(func(c *passCounts) { c.folders++ })
		return nil
	}
	if linkOnPath(s.opt.dir, rel) != "" {
		s.warnOnce("link:"+rel, "skipped: a symbolic link is in the way of the folder", "folder", rel)
		return nil
	}
	if err := os.MkdirAll(local, 0755); err != nil {
		s.warnOnce("mkdir:"+rel, "skipped: the folder cannot be made here", "folder", rel, "error", err.Error())
		return nil
	}
	if m.mode != 0 {
		if err := os.Chmod(local, m.mode); err != nil {
			return err
		}
	}
	info, err := os.Lstat(local)
	if err != nil {
		return nil
	}
	d := newLocalDir(rel+"/", info)
	s.mu.Lock()
	s.dirs[a.id] = d
	s.counts.folders++
	s.mu.Unlock()
	s.log.Info("made folder", "folder", rel)
	s.agreeDir(a.id, d, m)
	return nil
}

// chmodDir gives a local folder its marker's mode.
func (s *syncer) chmodDir(a action) error {
	d, m := a.d, a.r
	rel := strings.TrimSuffix(d.rel, "/")
	if s.opt.dryRun {
		s.log.Info("would set mode", "folder", rel, "mode", fmt.Sprintf("%04o", m.mode))
		s.count(func(c *passCounts) { c.modes++ })
		return nil
	}
	local := filepath.Join(s.opt.dir, filepath.FromSlash(rel))
	if info, err := os.Lstat(local); err != nil || !info.IsDir() {
		return nil
	}
	if err := os.Chmod(local, m.mode); err != nil {
		return err
	}
	nd := *d
	nd.mode = m.mode
	s.mu.Lock()
	s.dirs[a.id] = &nd
	s.counts.modes++
	s.mu.Unlock()
	s.log.Info("set mode", "folder", rel, "mode", fmt.Sprintf("%04o", m.mode))
	s.agreeDir(a.id, &nd, m)
	return nil
}

// removeDir removes a local folder the bucket no longer has, when it is
// empty. One that is not, holding a file made since, is kept: two ways, its
// marker is written again.
func (s *syncer) removeDir(ctx context.Context, a action) error {
	rel := strings.TrimSuffix(a.d.rel, "/")
	local, err := within(s.opt.dir, rel)
	if err != nil {
		return err
	}
	if s.opt.dryRun {
		s.log.Info("would delete folder", "folder", rel)
		s.count(func(c *passCounts) { c.deleted++ })
		return nil
	}
	if err := os.Remove(local); err != nil && !os.IsNotExist(err) {
		if entries, rerr := os.ReadDir(local); rerr != nil || len(entries) == 0 {
			return err
		}
		s.log.Debug("not deleting a folder that is not empty", "folder", rel)
		if s.opt.mode == modeTwoWay {
			return s.writeMarker(ctx, a.id, a.d, nil)
		}
		return nil
	}
	s.log.Info("deleted folder", "folder", rel)
	s.mu.Lock()
	delete(s.dirs, a.id)
	if s.opt.mode == modeTwoWay {
		delete(s.base, a.id)
		s.baseDirt = true
	}
	s.counts.deleted++
	s.mu.Unlock()
	return nil
}

// deleteMarker removes a folder's marker from the bucket.
func (s *syncer) deleteMarker(ctx context.Context, a action) error {
	name := remoteName(s.opt.bucket, a.r.key)
	if s.opt.dryRun {
		s.log.Info("would delete folder", "folder", name)
		s.count(func(c *passCounts) { c.deleted++ })
		return nil
	}
	var err error
	if a.r.etag != "" {
		err = s.client.DeleteFileObjectIfMatch(ctx, s.opt.bucket, a.r.key, a.r.etag)
	} else {
		err = s.client.DeleteFileObject(ctx, s.opt.bucket, a.r.key)
	}
	if err != nil {
		if apiclient.IsPreconditionFailed(err) {
			s.log.Debug("changed in the bucket meanwhile: looking again", "folder", name)
			return nil
		}
		if !strings.Contains(err.Error(), "404") {
			return fmt.Errorf("%s: %w", name, cmdutil.CleanErr(err))
		}
	}
	s.log.Info("deleted folder", "folder", name)
	s.mu.Lock()
	delete(s.markers, a.id)
	if s.opt.mode == modeTwoWay {
		delete(s.base, a.id)
		s.baseDirt = true
	}
	s.counts.deleted++
	s.mu.Unlock()
	return nil
}
