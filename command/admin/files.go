package commands_admin

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/paularlott/knot/internal/filestore"
)

// conflictsDir holds exported files whose key cannot be written at its own
// path, named by content checksum. A bucket name never starts with a dot, so
// it cannot clash with a bucket's directory.
const conflictsDir = ".knot-conflicts"

// exportPath returns where a file is exported to, or "" when its key does
// not map onto a path: an empty or over-long segment, or a backslash, which
// some systems read as a separator.
func exportPath(dir string, o *filestore.Object) string {
	key := strings.TrimSuffix(o.Key, "/")
	if key == "" || strings.ContainsRune(key, '\\') {
		return ""
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || len(part) > 255 {
			return ""
		}
	}
	return filepath.Join(dir, o.Bucket, filepath.FromSlash(key))
}

func conflictPath(dir, sha string) string {
	return filepath.Join(dir, conflictsDir, sha)
}

// exportFiles copies the content of every file in d from the storage
// directory into dir as <bucket>/<key>. Files whose key cannot be written
// there are put in the conflicts directory, listed in its index.
func exportFiles(filesPath, dir string, d *filestore.BackupData) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("the files directory %s must be empty", dir)
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	for _, b := range d.Buckets {
		if err := os.MkdirAll(filepath.Join(dir, b.Name), 0755); err != nil {
			return err
		}
	}

	// Shorter keys first, so a folder's files land beneath it.
	objects := append([]*filestore.Object(nil), d.Objects...)
	sort.Slice(objects, func(i, j int) bool {
		if objects[i].Bucket != objects[j].Bucket {
			return objects[i].Bucket < objects[j].Bucket
		}
		return objects[i].Key < objects[j].Key
	})

	var index bytes.Buffer
	var exported, conflicts, missing int
	for _, o := range objects {
		path := exportPath(dir, o)

		// An empty key ending in / marks a folder.
		if path != "" && strings.HasSuffix(o.Key, "/") && o.Size == 0 {
			if os.MkdirAll(path, 0755) == nil {
				exported++
				continue
			}
			path = ""
		} else if strings.HasSuffix(o.Key, "/") {
			path = ""
		}

		if path != "" {
			err := exportOne(filesPath, o, path, false)
			if err == nil {
				exported++
				continue
			}
			if err == errNoContent {
				fmt.Printf("Warning: content of %s/%s is not stored on this server\n", o.Bucket, o.Key)
				missing++
				continue
			}
			// Anything else is something in the way of the path, such as a
			// file where a folder is needed; a real write failure fails again
			// below.
		}

		// The key collides with another file or folder, or is not a path.
		fmt.Fprintf(&index, "%s\t%s/%s\n", o.SHA256, o.Bucket, o.Key)
		if err := os.MkdirAll(filepath.Join(dir, conflictsDir), 0755); err != nil {
			return err
		}
		if err := exportOne(filesPath, o, conflictPath(dir, o.SHA256), true); err != nil {
			if err == errNoContent {
				fmt.Printf("Warning: content of %s/%s is not stored on this server\n", o.Bucket, o.Key)
				missing++
				continue
			}
			return fmt.Errorf("Error exporting %s/%s: %w", o.Bucket, o.Key, err)
		}
		conflicts++
	}

	if index.Len() > 0 {
		if err := os.WriteFile(filepath.Join(dir, conflictsDir, "index.txt"), index.Bytes(), 0644); err != nil {
			return err
		}
	}
	fmt.Printf("Exported %d files", exported+conflicts)
	if conflicts > 0 {
		fmt.Printf(", %d of them to %s as their names cannot be used as paths", conflicts, conflictsDir)
	}
	fmt.Println()
	if missing > 0 {
		return fmt.Errorf("%d files have no content on this server; back up a server that holds them", missing)
	}
	return nil
}

// exportOne writes one file's content to path. Unless shared content may
// already be there, path must not exist, so keys that differ only in case on
// a case-insensitive system are caught. Content is opened before anything is
// created, so missing content leaves nothing behind.
func exportOne(filesPath string, o *filestore.Object, path string, mayExist bool) error {
	flags := os.O_CREATE | os.O_WRONLY | os.O_EXCL
	if mayExist {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	src, err := filestore.OpenContent(filesPath, o.SHA256)
	if err != nil {
		return errNoContent
	}
	defer src.Close()

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	dst, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		os.Remove(path)
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if !o.ModifiedAt.IsZero() {
		os.Chtimes(path, o.ModifiedAt, o.ModifiedAt)
	}
	return nil
}

var errNoContent = errors.New("content not stored on this server")

// importFiles stores the content of every file in d that the store lacks,
// read from an export made by exportFiles. Files it finds no content for are
// reported; in a cluster the server fetches them from the others.
func importFiles(store *filestore.Store, dir string, d *filestore.BackupData) {
	var imported, missing int
	for _, o := range d.Objects {
		if store.HasContent(o.SHA256) {
			continue
		}
		if err := importOne(store, dir, o); err != nil {
			fmt.Printf("Warning: no content for %s/%s in %s: %v\n", o.Bucket, o.Key, dir, err)
			missing++
			continue
		}
		imported++
	}
	fmt.Printf("Imported the content of %d files\n", imported)
	if missing > 0 {
		fmt.Printf("Warning: %d files have no content; in a cluster they are fetched from the other servers\n", missing)
	}
}

func importOne(store *filestore.Store, dir string, o *filestore.Object) error {
	if o.Size == 0 {
		return store.ImportContent(bytes.NewReader(nil), o.SHA256)
	}
	err := os.ErrNotExist
	for _, path := range []string{exportPath(dir, o), conflictPath(dir, o.SHA256)} {
		if path == "" {
			continue
		}
		var f *os.File
		if f, err = os.Open(path); err != nil {
			continue
		}
		err = store.ImportContent(f, o.SHA256)
		f.Close()
		if err == nil {
			return nil
		}
	}
	return err
}

// filesOwnedBy keeps only the buckets owned by a user, and their files.
func filesOwnedBy(d *filestore.BackupData, userId string) *filestore.BackupData {
	out := &filestore.BackupData{}
	keep := make(map[string]bool)
	for _, b := range d.Buckets {
		if b.OwnerId == userId {
			out.Buckets = append(out.Buckets, b)
			keep[b.Name] = true
		}
	}
	for _, o := range d.Objects {
		if keep[o.Bucket] {
			out.Objects = append(out.Objects, o)
		}
	}
	return out
}
