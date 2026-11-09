package backupfile

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
)

// Content is the file content of a backup folder. Without a key it is kept
// as it is, under its checksum. With a key it is encrypted, so a folder that
// leaves the server gives away neither what its files hold nor, by their
// names, whether it holds a file someone already has:
//
//   - each content file is named by an HMAC of its checksum under a key
//     derived from the backup key, rather than by the checksum;
//   - its content is encrypted with AES-256-GCM under a key derived from the
//     backup key and its checksum, in chunks of 64 KiB, each checked on its
//     own. A chunk's nonce is its number, with the last one marked, so a
//     file cut short, or with its chunks in another order, does not decrypt.
//
// The same content always encrypts the same way under the same key, so
// content shared by many files is still kept once, and a backup repeated
// into the same folder copies only what is new. A key is never used for two
// different contents, so a nonce is never used twice under one key.
//
// An encrypted content file is a header, magic and version, then its chunks,
// each its ciphertext and tag:
//
//	"KNBC" 1  chunk... (64 KiB + 16 bytes each, the last shorter)
type Content struct {
	dir  string
	keys *contentKeys // nil when the content is kept as it is
}

const (
	contentMagic   = "KNBC"
	contentVersion = 1
	contentHeader  = len(contentMagic) + 1
	chunkSize      = 64 << 10
	chunkTag       = 16
	sealedChunk    = chunkSize + chunkTag
)

type contentKeys struct {
	master []byte
	names  []byte
}

// NewContent is the content of the backup in dir: encrypted with key when
// encrypted is set, as the manifest's ContentEncrypted says or, for a new
// backup, whenever there is a key.
func NewContent(dir, key string, encrypted bool) (*Content, error) {
	c := &Content{dir: dir}
	if !encrypted {
		return c, nil
	}
	if len(key) != 32 {
		return nil, errors.New("the encryption key must be 32 bytes long")
	}
	names, err := hkdf.Key(sha256.New, []byte(key), nil, "knot backup content names", 32)
	if err != nil {
		return nil, err
	}
	c.keys = &contentKeys{master: []byte(key), names: names}
	return c, nil
}

// Encrypted reports whether the content is encrypted.
func (c *Content) Encrypted() bool { return c.keys != nil }

// Name is the name the content with a checksum is kept under.
func (c *Content) Name(sha string) string {
	if c.keys == nil {
		return sha
	}
	raw, _ := hex.DecodeString(sha)
	m := hmac.New(sha256.New, c.keys.names)
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}

// path is where the content with a checksum is kept.
func (c *Content) path(sha string) string { return ContentPath(c.dir, c.Name(sha)) }

// aead is the cipher for the content with a checksum.
func (c *Content) aead(sha string) (cipher.AEAD, error) {
	raw, err := hex.DecodeString(sha)
	if err != nil {
		return nil, err
	}
	key, err := hkdf.Key(sha256.New, c.keys.master, raw, "knot backup content", 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func chunkNonce(n uint64, last bool) []byte {
	nonce := make([]byte, 12)
	binary.BigEndian.PutUint64(nonce, n)
	if last {
		nonce[11] = 1
	}
	return nonce
}

// StoredSize is the size, in the folder, of content of size bytes.
func (c *Content) StoredSize(size int64) int64 {
	if c.keys == nil {
		return size
	}
	chunks := max(1, (size+chunkSize-1)/chunkSize)
	return int64(contentHeader) + size + chunks*chunkTag
}

// plainSize is the size of the content an encrypted file of stored bytes
// holds, false when no content gives that size.
func plainSize(stored int64) (int64, bool) {
	body := stored - int64(contentHeader)
	if body < chunkTag {
		return 0, false
	}
	chunks := (body + sealedChunk - 1) / sealedChunk
	size := body - chunks*chunkTag
	return size, size >= 0
}

// Has reports whether the backup holds the content with a checksum.
func (c *Content) Has(sha string) bool {
	if len(sha) != 64 {
		return false
	}
	_, err := os.Stat(c.path(sha))
	return err == nil
}

// HasOfSize is Has that also checks the size, so content a crash left empty
// or cut short is copied again rather than trusted. Content is not synced
// file by file, which would make a backup of many small files crawl.
func (c *Content) HasOfSize(sha string, size int64) bool {
	if len(sha) != 64 {
		return false
	}
	info, err := os.Stat(c.path(sha))
	return err == nil && info.Size() == c.StoredSize(size)
}

// Open opens the content with a checksum, returning it as it was backed up
// and its size. Encrypted content is decrypted as it is read: a chunk that
// does not decrypt, or a file cut short, fails the read.
func (c *Content) Open(sha string) (io.ReadCloser, int64, error) {
	if len(sha) != 64 {
		return nil, 0, os.ErrNotExist
	}
	f, err := os.Open(c.path(sha))
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	if c.keys == nil {
		return f, info.Size(), nil
	}
	size, ok := plainSize(info.Size())
	aead, err := c.aead(sha)
	if err != nil || !ok {
		f.Close()
		return nil, 0, damaged(sha)
	}
	head := make([]byte, contentHeader)
	if _, err := io.ReadFull(f, head); err != nil || string(head[:len(contentMagic)]) != contentMagic || head[len(contentMagic)] != contentVersion {
		f.Close()
		return nil, 0, damaged(sha)
	}
	return &decrypter{f: f, r: bufio.NewReaderSize(f, sealedChunk), aead: aead, sha: sha, left: info.Size() - int64(contentHeader)}, size, nil
}

func damaged(sha string) error {
	return fmt.Errorf("the backup's copy of %s does not decrypt: wrong key, or it is damaged", sha[:12])
}

// decrypter reads encrypted content, a chunk at a time.
type decrypter struct {
	f    *os.File
	r    *bufio.Reader
	aead cipher.AEAD
	sha  string
	left int64 // stored bytes still to read
	n    uint64
	buf  []byte // decrypted, not yet read
	done bool
}

func (d *decrypter) Read(p []byte) (int, error) {
	for len(d.buf) == 0 {
		if d.done {
			return 0, io.EOF
		}
		if err := d.next(); err != nil {
			return 0, err
		}
	}
	n := copy(p, d.buf)
	d.buf = d.buf[n:]
	return n, nil
}

func (d *decrypter) next() error {
	size := min(int64(sealedChunk), d.left)
	last := size == d.left
	sealed := make([]byte, size)
	if _, err := io.ReadFull(d.r, sealed); err != nil {
		return damaged(d.sha)
	}
	plain, err := d.aead.Open(sealed[:0], chunkNonce(d.n, last), sealed, nil)
	if err != nil {
		return damaged(d.sha)
	}
	d.left -= size
	d.n++
	d.buf, d.done = plain, last
	return nil
}

func (d *decrypter) Close() error { return d.f.Close() }

// Download copies the content with a checksum into the backup, carrying on
// from a partial copy if there is one, and checking the whole against the
// checksum before keeping it. A failure leaves the partial copy for the next
// attempt; content that fails its checksum is discarded. It returns the
// bytes of content copied in this call.
func (c *Content) Download(sha string, fetch Fetch) (int64, error) {
	if c.keys == nil {
		return DownloadContent(c.dir, sha, fetch)
	}
	if len(sha) != 64 {
		return 0, errors.New("invalid checksum")
	}
	aead, err := c.aead(sha)
	if err != nil {
		return 0, err
	}
	name := c.Name(sha)
	dst, part := ContentPath(c.dir, name), PartialPath(c.dir, name)
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

	// Keep what a partial copy holds that decrypts, whole chunks, and carry
	// on after it.
	h := sha256.New()
	chunks, whole, err := readPartial(f, aead, h)
	if err != nil {
		return 0, err
	}
	finish := func() error {
		f.Close()
		closed = true
		if hex.EncodeToString(h.Sum(nil)) != sha {
			os.Remove(part)
			return fmt.Errorf("content does not match its checksum %s", sha)
		}
		return os.Rename(part, dst)
	}
	if whole {
		// It died only before its rename.
		return 0, finish()
	}
	offset := int64(chunks) * chunkSize
	rc, resumed, err := fetch(offset)
	if err != nil {
		return 0, err
	}
	defer rc.Close()
	if offset > 0 && !resumed {
		// The server sent it all again.
		h.Reset()
		chunks, offset = 0, 0
	}
	if err := f.Truncate(int64(contentHeader) + int64(chunks)*sealedChunk); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, err
	}
	if _, err := f.Write(append([]byte(contentMagic), contentVersion)); err != nil {
		return 0, err
	}
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return 0, err
	}

	// Encrypt what comes, a chunk at a time, writing only whole chunks: one
	// cut short is fetched again next time. Whether a chunk is the last is
	// known by looking for more after it.
	in := bufio.NewReaderSize(rc, chunkSize)
	w := bufio.NewWriterSize(f, 4*sealedChunk)
	buf := make([]byte, chunkSize)
	sealed := make([]byte, 0, sealedChunk)
	var copied int64
	n := uint64(chunks)
	for {
		got, last, err := readChunk(in, buf)
		if err != nil {
			w.Flush()
			return copied, err
		}
		h.Write(buf[:got])
		sealed = aead.Seal(sealed[:0], chunkNonce(n, last), buf[:got], nil)
		if _, err := w.Write(sealed); err != nil {
			return copied, err
		}
		copied += int64(got)
		n++
		if last {
			break
		}
	}
	if err := w.Flush(); err != nil {
		return copied, err
	}
	return copied, finish()
}

// readChunk fills buf from r, reporting whether the stream ended with it.
// Only a clean end is one: a connection cut short (which an HTTP body
// reports as io.ErrUnexpectedEOF, as io.ReadFull does a short read) is an
// error, so the chunk is fetched again rather than taken as the last.
func readChunk(r *bufio.Reader, buf []byte) (int, bool, error) {
	got := 0
	for got < len(buf) {
		n, err := r.Read(buf[got:])
		got += n
		if err == io.EOF {
			return got, true, nil
		}
		if err != nil {
			return got, false, err
		}
	}
	if _, err := r.Peek(1); err == io.EOF {
		return got, true, nil
	} else if err != nil {
		return got, false, err
	}
	return got, false, nil
}

// readPartial reads the chunks of a partial copy that decrypt, adding their
// content to h, and reports how many there are and whether the last of them
// is the content's last. A partial copy that is not one is started again.
func readPartial(f *os.File, aead cipher.AEAD, h hash.Hash) (chunks int, whole bool, err error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return 0, false, err
	}
	r := bufio.NewReaderSize(f, sealedChunk)
	head := make([]byte, contentHeader)
	if _, err := io.ReadFull(r, head); err != nil || string(head[:len(contentMagic)]) != contentMagic || head[len(contentMagic)] != contentVersion {
		return 0, false, nil
	}
	sealed := make([]byte, sealedChunk)
	for {
		got, _ := io.ReadFull(r, sealed)
		if got < chunkTag {
			return chunks, false, nil
		}
		// A chunk read whole may be the last; a short one can only be.
		if got == sealedChunk {
			if plain, oerr := aead.Open(nil, chunkNonce(uint64(chunks), false), sealed, nil); oerr == nil {
				h.Write(plain)
				chunks++
				continue
			}
		}
		if plain, oerr := aead.Open(nil, chunkNonce(uint64(chunks), true), sealed[:got], nil); oerr == nil {
			if _, perr := r.Peek(1); perr == io.EOF {
				h.Write(plain)
				return chunks + 1, true, nil
			}
		}
		return chunks, false, nil
	}
}

// Prune removes content that no file refers to: wanted holds the checksums
// still referred to. It also clears partial copies, and content kept under
// another key or none, which no file of this backup can be read from. It
// returns how many files it removed and the bytes they held.
func (c *Content) Prune(wanted map[string]struct{}) (int, int64, error) {
	names := make(map[string]struct{}, len(wanted))
	for sha := range wanted {
		names[c.Name(sha)] = struct{}{}
	}
	return Prune(c.dir, func(name string) bool { _, ok := names[name]; return ok })
}
