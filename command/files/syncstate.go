package command_files

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// A two-way sync keeps, between runs, the state both sides last agreed on:
// for each file, its content in the bucket and its size and modification
// time here. Without it a file deleted on one side looks the same as one
// created on the other. It lives in the user's configuration directory, not
// in the synced directory, so it is never synced itself.

const baseVersion = 1

type baseEntry struct {
	Key        string `json:"key"`         // the file's key in the bucket
	SHA        string `json:"sha256"`      // the content both sides held
	Size       int64  `json:"size"`        // its size
	LocalSize  int64  `json:"local_size"`  // the local file's size then
	LocalMtime int64  `json:"local_mtime"` // and its modification time, unix nanoseconds
}

type baseFile struct {
	Version int                   `json:"version"`
	Server  string                `json:"server"`
	Bucket  string                `json:"bucket"`
	Prefix  string                `json:"prefix"`
	Dir     string                `json:"dir"`
	Files   map[string]*baseEntry `json:"files"` // by identity
}

// defaultStatePath is where the state of a two-way sync of a bucket folder
// and a directory is kept.
func defaultStatePath(server, bucket, prefix, dir string) (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(server + "\x00" + bucket + "\x00" + prefix + "\x00" + dir))
	return filepath.Join(cfg, "knot", "sync", hex.EncodeToString(sum[:12])+".json"), nil
}

// loadBase reads the state of an earlier run. A missing file is a first run:
// nothing is deleted on it, and files that differ are kept both ways.
func (s *syncer) loadBase(bucketName string) error {
	if s.opt.statePath == "" {
		p, err := defaultStatePath(s.client.GetBaseURL(), bucketName, s.opt.prefix, s.opt.dir)
		if err != nil {
			return err
		}
		s.opt.statePath = p
	}
	data, err := os.ReadFile(s.opt.statePath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var bf baseFile
	if err := json.Unmarshal(data, &bf); err != nil {
		return fmt.Errorf("sync state %s is damaged (%v): remove it to start afresh", s.opt.statePath, err)
	}
	if bf.Version != baseVersion {
		return fmt.Errorf("sync state %s is from another version of knot: remove it to start afresh", s.opt.statePath)
	}
	if bf.Dir != s.opt.dir || bf.Prefix != s.opt.prefix || bf.Bucket != bucketName {
		return fmt.Errorf("sync state %s belongs to %s and %s:%s", s.opt.statePath, bf.Dir, bf.Bucket, bf.Prefix)
	}
	if bf.Files != nil {
		s.base = bf.Files
	}
	return nil
}

// saveBase writes the state when it changed, replacing the file at once so a
// crash leaves the old state or the new, never part of one.
func (s *syncer) saveBase() error {
	if s.opt.mode != modeTwoWay || s.opt.dryRun {
		return nil
	}
	s.mu.Lock()
	if !s.baseDirt {
		s.mu.Unlock()
		return nil
	}
	bucketName := s.opt.bucket
	if b, err := s.bucketName(); err == nil {
		bucketName = b
	}
	data, err := json.Marshal(&baseFile{
		Version: baseVersion,
		Server:  s.client.GetBaseURL(),
		Bucket:  bucketName,
		Prefix:  s.opt.prefix,
		Dir:     s.opt.dir,
		Files:   s.base,
	})
	s.baseDirt = false
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.opt.statePath), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.opt.statePath), ".sync-state-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), s.opt.statePath); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

func (s *syncer) bucketName() (string, error) {
	if s.fullName == "" {
		return "", fmt.Errorf("bucket not resolved")
	}
	return s.fullName, nil
}
