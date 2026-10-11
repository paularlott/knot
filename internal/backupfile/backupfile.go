// Package backupfile reads and writes the folder a backup is kept in:
//
//	manifest.json      what the backup holds, written last
//	<kind>.jsonl       the records of one kind, a JSON line each
//	content/aa/bb/<sha>  file content, by its checksum
//
// A record file begins with a header line. With a key the records follow in
// encrypted blocks of about a thousand lines, so a backup of any size is read
// and written a block at a time. Content is stored as it is, once however many
// files share it, so repeating a backup into the same folder copies only what
// is new.
package backupfile

import (
	"bufio"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Version is the folder format.
	Version = 1

	blockLines = 1000
	blockBytes = 1 << 20
)

// ErrWrongKey is returned when a block does not decrypt.
var ErrWrongKey = errors.New("cannot decrypt the backup: wrong key, or the file is damaged")

// Manifest says what a backup holds. It is written when the backup is
// complete, so a folder without one is unfinished.
type Manifest struct {
	Version   int            `json:"version"`
	Created   time.Time      `json:"created"`
	Server    string         `json:"server"` // the knot version that made it
	Encrypted bool           `json:"encrypted"`
	Counts    map[string]int `json:"counts"`
	Content   bool           `json:"content"` // file content was copied
}

// WriteManifest writes the manifest into dir.
func WriteManifest(dir string, m *Manifest) error {
	m.Version = Version
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, "manifest.json.tmp")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(dir, "manifest.json"))
}

// unfinishedFile marks a folder whose backup has begun but not completed, so
// a backup stopped part way can be run again into the same folder.
const unfinishedFile = "backup.unfinished"

// MarkUnfinished records that a backup into dir has begun.
func MarkUnfinished(dir string) error {
	return os.WriteFile(filepath.Join(dir, unfinishedFile), nil, 0600)
}

// ClearUnfinished removes the mark once the backup is complete.
func ClearUnfinished(dir string) error {
	err := os.Remove(filepath.Join(dir, unfinishedFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// IsBackupDir reports whether dir holds a backup, finished or not.
func IsBackupDir(dir string) bool {
	for _, f := range []string{"manifest.json", unfinishedFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
			return true
		}
	}
	return false
}

// ReadManifest reads the manifest of the backup in dir.
func ReadManifest(dir string) (*Manifest, error) {
	// The mark is also set while a refreshed backup swaps in its records, when
	// the manifest it still holds describes the old ones.
	if _, err := os.Stat(filepath.Join(dir, unfinishedFile)); err == nil {
		return nil, fmt.Errorf("%s holds an unfinished backup; run the backup again to finish it", dir)
	}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s holds no complete backup: it has no manifest.json; run the backup again to finish it", dir)
	}
	if err != nil {
		return nil, err
	}
	m := &Manifest{}
	if err := json.Unmarshal(data, m); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	if m.Version != Version {
		return nil, fmt.Errorf("the backup is format version %d, this knot reads version %d", m.Version, Version)
	}
	return m, nil
}

// RecordPath is the file the records of a kind are kept in.
func RecordPath(dir, kind string) string { return filepath.Join(dir, kind+".jsonl") }

type header struct {
	Knot      int    `json:"knot_backup"`
	Kind      string `json:"kind"`
	Encrypted bool   `json:"encrypted"`
}

func newGCM(key string) (cipher.AEAD, error) {
	if len(key) != 32 {
		return nil, errors.New("the encryption key must be 32 bytes long")
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Writer writes the records of one kind.
type Writer struct {
	f     *os.File
	bw    *bufio.Writer
	kind  string
	gcm   cipher.AEAD // nil when not encrypted
	buf   bytes.Buffer
	lines int
	block int
	n     int
}

// Create starts the record file of a kind in dir. With a key the records are
// encrypted.
func Create(dir, kind, key string) (*Writer, error) {
	w := &Writer{kind: kind}
	if key != "" {
		var err error
		if w.gcm, err = newGCM(key); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(RecordPath(dir, kind), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	w.f = f
	w.bw = bufio.NewWriterSize(f, 256*1024)
	line, _ := json.Marshal(header{Knot: Version, Kind: kind, Encrypted: key != ""})
	w.bw.Write(append(line, '\n'))
	return w, nil
}

// Add writes a record, a JSON value with no newline in it.
func (w *Writer) Add(line []byte) error {
	w.n++
	if w.gcm == nil {
		if _, err := w.bw.Write(line); err != nil {
			return err
		}
		return w.bw.WriteByte('\n')
	}
	if w.lines > 0 {
		w.buf.WriteByte('\n')
	}
	w.buf.Write(line)
	w.lines++
	if w.lines >= blockLines || w.buf.Len() >= blockBytes {
		return w.flushBlock()
	}
	return nil
}

func (w *Writer) aad() []byte { return []byte(fmt.Sprintf("%s/%d", w.kind, w.block)) }

func (w *Writer) flushBlock() error {
	if w.lines == 0 {
		return nil
	}
	nonce := make([]byte, w.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := w.gcm.Seal(nonce, nonce, w.buf.Bytes(), w.aad())
	if _, err := w.bw.WriteString(base64.StdEncoding.EncodeToString(sealed)); err != nil {
		return err
	}
	if err := w.bw.WriteByte('\n'); err != nil {
		return err
	}
	w.buf.Reset()
	w.lines = 0
	w.block++
	return nil
}

// Count is how many records have been added.
func (w *Writer) Count() int { return w.n }

// Close finishes the file, making it durable.
func (w *Writer) Close() error {
	if w.gcm != nil {
		if err := w.flushBlock(); err != nil {
			w.f.Close()
			return err
		}
	}
	if err := w.bw.Flush(); err != nil {
		w.f.Close()
		return err
	}
	if err := w.f.Sync(); err != nil {
		w.f.Close()
		return err
	}
	return w.f.Close()
}

// Reader reads the records of one kind.
type Reader struct {
	f    *os.File
	br   *bufio.Reader
	kind string
	gcm  cipher.AEAD
}

// Open opens the record file of a kind in dir; key is needed if it is
// encrypted. A kind the backup has no file for returns os.ErrNotExist.
func Open(dir, kind, key string) (*Reader, error) {
	f, err := os.Open(RecordPath(dir, kind))
	if err != nil {
		return nil, err
	}
	r := &Reader{f: f, br: bufio.NewReaderSize(f, 256*1024), kind: kind}
	line, err := r.br.ReadBytes('\n')
	var h header
	if err != nil || json.Unmarshal(line, &h) != nil || h.Knot != Version || h.Kind != kind {
		f.Close()
		return nil, fmt.Errorf("%s is not a knot backup file of %s", RecordPath(dir, kind), kind)
	}
	if h.Encrypted {
		if key == "" {
			f.Close()
			return nil, fmt.Errorf("%s is encrypted: give the key with --encrypt-key", RecordPath(dir, kind))
		}
		if r.gcm, err = newGCM(key); err != nil {
			f.Close()
			return nil, err
		}
	}
	return r, nil
}

// Each calls fn with every record, in order, stopping at the first error.
func (r *Reader) Each(fn func(line []byte) error) error {
	block := 0
	for {
		line, err := r.br.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\r\n")
			if r.gcm == nil {
				if len(line) > 0 {
					if ferr := fn(line); ferr != nil {
						return ferr
					}
				}
			} else if len(line) > 0 {
				sealed, derr := base64.StdEncoding.DecodeString(string(line))
				ns := r.gcm.NonceSize()
				if derr != nil || len(sealed) < ns {
					return ErrWrongKey
				}
				plain, derr := r.gcm.Open(nil, sealed[:ns], sealed[ns:], []byte(fmt.Sprintf("%s/%d", r.kind, block)))
				if derr != nil {
					return ErrWrongKey
				}
				block++
				for _, l := range bytes.Split(plain, []byte{'\n'}) {
					if len(l) == 0 {
						continue
					}
					if ferr := fn(l); ferr != nil {
						return ferr
					}
				}
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func (r *Reader) Close() { r.f.Close() }

// ---------------------------------------------------------------------------
// Content
// ---------------------------------------------------------------------------

// ContentPath is where the content with a checksum is kept.
func ContentPath(dir, sha string) string {
	return filepath.Join(dir, "content", sha[0:2], sha[2:4], sha)
}

// HasContent reports whether the backup holds the content with a checksum.
func HasContent(dir, sha string) bool {
	if len(sha) != 64 {
		return false
	}
	_, err := os.Stat(ContentPath(dir, sha))
	return err == nil
}

// HasContentOfSize is HasContent that also checks the size, so content a crash
// left empty or cut short is copied again rather than trusted. Content is not
// synced file by file, which would make a backup of many small files crawl.
func HasContentOfSize(dir, sha string, size int64) bool {
	if len(sha) != 64 {
		return false
	}
	info, err := os.Stat(ContentPath(dir, sha))
	return err == nil && info.Size() == size
}

// WriteContent stores content read from r under its checksum, refusing
// content that does not match it. It returns the bytes written.
func WriteContent(dir, sha string, r io.Reader) (int64, error) {
	if len(sha) != 64 {
		return 0, errors.New("invalid checksum")
	}
	dst := ContentPath(dir, sha)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return 0, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".incoming-*")
	if err != nil {
		return 0, err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err == nil && hex.EncodeToString(h.Sum(nil)) != sha {
		err = fmt.Errorf("content does not match its checksum %s", sha)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return 0, err
	}
	return n, os.Rename(tmp.Name(), dst)
}

// OpenContent opens the content with a checksum.
func OpenContent(dir, sha string) (*os.File, error) {
	if len(sha) != 64 {
		return nil, os.ErrNotExist
	}
	return os.Open(ContentPath(dir, sha))
}

// PartialPath is where content with a checksum is kept while it is being
// copied. A copy that fails leaves it, so the next attempt, in this run or
// the next, carries on from there.
func PartialPath(dir, sha string) string {
	return filepath.Join(filepath.Dir(ContentPath(dir, sha)), ".partial-"+sha)
}

// Fetch opens content from byte offset. It returns the stream and whether the
// stream starts at offset: false means it starts at the beginning, because the
// server sent everything again.
type Fetch func(offset int64) (rc io.ReadCloser, resumed bool, err error)

// DownloadContent copies the content with a checksum into the backup, carrying
// on from a partial copy if there is one, and checking the whole against the
// checksum before keeping it. A failure leaves the partial copy for the next
// attempt; content that fails its checksum is discarded. It returns the bytes
// copied in this call.
func DownloadContent(dir, sha string, fetch Fetch) (int64, error) {
	if len(sha) != 64 {
		return 0, errors.New("invalid checksum")
	}
	dst, part := ContentPath(dir, sha), PartialPath(dir, sha)
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return 0, err
	}
	f, err := os.OpenFile(part, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return 0, err
	}
	closed := false
	defer func() {
		if !closed {
			f.Close()
		}
	}()

	offset, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, err
	}
	// A partial copy that died only before its rename is whole already, and
	// asking for bytes past its end would be refused.
	if offset > 0 {
		if sum, err := sumFile(f); err == nil && sum == sha {
			f.Close()
			closed = true
			return 0, os.Rename(part, dst)
		}
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			return 0, err
		}
	}
	rc, resumed, err := fetch(offset)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	if offset > 0 && !resumed {
		// The server sent it all again.
		if err := f.Truncate(0); err != nil {
			return 0, err
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return 0, err
		}
		offset = 0
	}
	n, err := io.Copy(f, rc)
	if err != nil {
		return n, err
	}

	// Check the whole, which may be part of an earlier attempt.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return n, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return n, err
	}
	f.Close()
	closed = true
	if hex.EncodeToString(h.Sum(nil)) != sha {
		os.Remove(part)
		return n, fmt.Errorf("content does not match its checksum %s", sha)
	}
	return n, os.Rename(part, dst)
}

// sumFile returns the SHA-256 of f from its start.
func sumFile(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Prune removes content that no file refers to: keep reports whether a
// checksum is still wanted. It also clears partial copies. It returns how many
// files it removed and the bytes they held.
func Prune(dir string, keep func(sha string) bool) (removed int, freed int64, err error) {
	root := filepath.Join(dir, "content")
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, werr error) error {
		if werr != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if len(name) == 64 && keep(name) {
			return nil
		}
		if len(name) != 64 && !strings.HasPrefix(name, ".partial-") && !strings.HasPrefix(name, ".incoming-") {
			return nil // not ours
		}
		if info, ierr := d.Info(); ierr == nil {
			freed += info.Size()
		}
		if os.Remove(path) == nil {
			removed++
		}
		return nil
	})
	if err != nil {
		return
	}
	// Empty folders.
	for _, a := range readDirs(root) {
		for _, b := range readDirs(a) {
			os.Remove(b)
		}
		os.Remove(a)
	}
	return
}

func readDirs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out
}
