package backupfile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testKey  = "0123456789abcdef0123456789abcdef"
	otherKey = "fedcba9876543210fedcba9876543210"
)

func sealed(t *testing.T, dir, key string) *Content {
	t.Helper()
	c, err := NewContent(dir, key, true)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func shaOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// serve fetches data from offset, as the server does.
func serve(data []byte) Fetch {
	return func(offset int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(bytes.NewReader(data[offset:])), true, nil
	}
}

func readAll(t *testing.T, c *Content, sha string) ([]byte, error) {
	t.Helper()
	r, size, err := c.Open(sha)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	got, err := io.ReadAll(r)
	if err == nil && int64(len(got)) != size {
		t.Errorf("Open said %d bytes, read %d", size, len(got))
	}
	return got, err
}

// Content of every size around a chunk's comes back as it went in, under a
// name and in a form that give nothing of it away.
func TestEncryptedContentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := sealed(t, dir, testKey)
	for _, size := range []int{0, 1, chunkSize - 1, chunkSize, chunkSize + 1, 3 * chunkSize, 3*chunkSize + 77} {
		data := bytes.Repeat([]byte("secret!"), size/7+1)[:size]
		sha := shaOf(data)
		n, err := c.Download(sha, serve(data))
		if err != nil || n != int64(size) {
			t.Fatalf("size %d: download %d %v", size, n, err)
		}
		if !c.Has(sha) || !c.HasOfSize(sha, int64(size)) || c.HasOfSize(sha, int64(size)+1) {
			t.Errorf("size %d: not held at its size", size)
		}
		stored, err := os.ReadFile(c.path(sha))
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(stored)) != c.StoredSize(int64(size)) {
			t.Errorf("size %d: stored %d bytes, StoredSize says %d", size, len(stored), c.StoredSize(int64(size)))
		}
		if size > 0 && bytes.Contains(stored, []byte("secret!")) {
			t.Errorf("size %d: stored in the clear", size)
		}
		if c.Name(sha) == sha || plain(dir).Has(sha) {
			t.Errorf("size %d: kept under its checksum", size)
		}
		got, err := readAll(t, c, sha)
		if err != nil || !bytes.Equal(got, data) {
			t.Errorf("size %d: read back %d bytes, %v", size, len(got), err)
		}
	}
	if entries, _ := filepath.Glob(filepath.Join(dir, "content", "*", "*", ".partial-*")); len(entries) != 0 {
		t.Errorf("partial copies left: %v", entries)
	}
}

// The same content encrypts the same way under the same key, so it is kept
// once and a repeated backup finds it; under another key it is another file
// under another name.
func TestEncryptedContentDeterministic(t *testing.T) {
	data := []byte(strings.Repeat("shared ", 20000))
	sha := shaOf(data)
	a, b, other := sealed(t, t.TempDir(), testKey), sealed(t, t.TempDir(), testKey), sealed(t, t.TempDir(), otherKey)
	for _, c := range []*Content{a, b, other} {
		if _, err := c.Download(sha, serve(data)); err != nil {
			t.Fatal(err)
		}
	}
	fa, _ := os.ReadFile(a.path(sha))
	fb, _ := os.ReadFile(b.path(sha))
	fo, _ := os.ReadFile(other.path(sha))
	if a.Name(sha) != b.Name(sha) || !bytes.Equal(fa, fb) {
		t.Error("the same content under the same key differs")
	}
	if a.Name(sha) == other.Name(sha) || bytes.Equal(fa, fo) {
		t.Error("another key gives the same name or content")
	}
}

// Content read with the wrong key, cut short, reordered or altered fails to
// read rather than giving back something else.
func TestEncryptedContentDamage(t *testing.T) {
	data := make([]byte, 3*chunkSize+100)
	rand.Read(data)
	sha := shaOf(data)

	setup := func(t *testing.T) (*Content, []byte) {
		c := sealed(t, t.TempDir(), testKey)
		if _, err := c.Download(sha, serve(data)); err != nil {
			t.Fatal(err)
		}
		stored, _ := os.ReadFile(c.path(sha))
		return c, stored
	}
	expectDamaged := func(t *testing.T, c *Content, stored []byte) {
		t.Helper()
		os.WriteFile(c.path(sha), stored, 0600)
		got, err := readAll(t, c, sha)
		if err == nil {
			t.Fatalf("read %d bytes of damaged content without an error", len(got))
		}
		if !strings.Contains(err.Error(), "does not decrypt") {
			t.Errorf("error %v", err)
		}
	}

	t.Run("wrong key", func(t *testing.T) {
		c, stored := setup(t)
		wrong := sealed(t, c.dir, otherKey)
		if wrong.Has(sha) {
			t.Fatal("found under another key's name")
		}
		os.MkdirAll(filepath.Dir(wrong.path(sha)), 0700)
		os.WriteFile(wrong.path(sha), stored, 0600)
		if _, err := readAll(t, wrong, sha); err == nil {
			t.Error("read with the wrong key")
		}
	})
	t.Run("last chunk dropped", func(t *testing.T) {
		c, stored := setup(t)
		expectDamaged(t, c, stored[:contentHeader+3*sealedChunk])
	})
	t.Run("cut part way through a chunk", func(t *testing.T) {
		c, stored := setup(t)
		expectDamaged(t, c, stored[:contentHeader+sealedChunk+500])
	})
	t.Run("chunks swapped", func(t *testing.T) {
		c, stored := setup(t)
		swapped := append([]byte{}, stored[:contentHeader]...)
		swapped = append(swapped, stored[contentHeader+sealedChunk:contentHeader+2*sealedChunk]...)
		swapped = append(swapped, stored[contentHeader:contentHeader+sealedChunk]...)
		swapped = append(swapped, stored[contentHeader+2*sealedChunk:]...)
		expectDamaged(t, c, swapped)
	})
	t.Run("a byte changed", func(t *testing.T) {
		c, stored := setup(t)
		stored[contentHeader+2*sealedChunk+10] ^= 1
		expectDamaged(t, c, stored)
	})
	t.Run("extra chunk", func(t *testing.T) {
		c, stored := setup(t)
		expectDamaged(t, c, append(stored, stored[contentHeader:contentHeader+sealedChunk]...))
	})
}

// A copy that breaks off carries on from its last whole chunk, asking the
// server for the rest from there.
func TestEncryptedContentResumes(t *testing.T) {
	data := make([]byte, 5*chunkSize+1234)
	rand.Read(data)
	sha := shaOf(data)
	c := sealed(t, t.TempDir(), testKey)

	_, err := c.Download(sha, func(offset int64) (io.ReadCloser, bool, error) {
		cut := data[:2*chunkSize+chunkSize/2]
		return io.NopCloser(io.MultiReader(bytes.NewReader(cut), errReader{io.ErrUnexpectedEOF})), true, nil
	})
	if err == nil || c.Has(sha) {
		t.Fatalf("a broken copy: %v", err)
	}
	var asked int64 = -1
	n, err := c.Download(sha, func(offset int64) (io.ReadCloser, bool, error) {
		asked = offset
		return io.NopCloser(bytes.NewReader(data[offset:])), true, nil
	})
	if err != nil || asked != 2*chunkSize || n != int64(len(data))-2*chunkSize {
		t.Fatalf("resumed at %d, copied %d: %v", asked, n, err)
	}
	if got, err := readAll(t, c, sha); err != nil || !bytes.Equal(got, data) {
		t.Errorf("resumed copy reads back wrong: %v", err)
	}

	// The server sending it all again rather than from the offset.
	c2 := sealed(t, t.TempDir(), testKey)
	c2.Download(sha, func(int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(io.MultiReader(bytes.NewReader(data[:3*chunkSize]), errReader{io.ErrUnexpectedEOF})), true, nil
	})
	if _, err := c2.Download(sha, func(int64) (io.ReadCloser, bool, error) {
		return io.NopCloser(bytes.NewReader(data)), false, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, err := readAll(t, c2, sha); err != nil || !bytes.Equal(got, data) {
		t.Errorf("copy sent again reads back wrong: %v", err)
	}

	// A copy that died only before its rename is kept without asking.
	c3 := sealed(t, t.TempDir(), testKey)
	c3.Download(sha, serve(data))
	name := c3.Name(sha)
	os.Rename(ContentPath(c3.dir, name), PartialPath(c3.dir, name))
	if _, err := c3.Download(sha, func(int64) (io.ReadCloser, bool, error) {
		return nil, false, errors.New("asked for content already held")
	}); err != nil || !c3.HasOfSize(sha, int64(len(data))) {
		t.Errorf("whole partial copy: %v", err)
	}

	// Content that does not match its checksum is not kept.
	c4 := sealed(t, t.TempDir(), testKey)
	if _, err := c4.Download(sha, serve([]byte("something else"))); err == nil || c4.Has(sha) {
		t.Errorf("mismatched content kept: %v", err)
	}
	if _, err := os.Stat(PartialPath(c4.dir, c4.Name(sha))); !os.IsNotExist(err) {
		t.Error("mismatched content left a partial copy")
	}
}

// Pruning keeps what files refer to, under this key's names, and removes
// the rest: content of another key or none, and partial copies.
func TestEncryptedContentPrune(t *testing.T) {
	dir := t.TempDir()
	c := sealed(t, dir, testKey)
	keep, drop := []byte("kept content"), []byte("dropped content")
	c.Download(shaOf(keep), serve(keep))
	c.Download(shaOf(drop), serve(drop))
	putContent(dir, shaOf(keep), string(keep)) // a plain copy, from a backup without a key
	half := PartialPath(dir, c.Name(shaOf([]byte("half"))))
	os.MkdirAll(filepath.Dir(half), 0700)
	os.WriteFile(half, []byte("x"), 0600)

	removed, _, err := c.Prune(map[string]struct{}{shaOf(keep): {}})
	if err != nil || removed != 3 {
		t.Errorf("removed %d: %v", removed, err)
	}
	if !c.Has(shaOf(keep)) || c.Has(shaOf(drop)) || plain(dir).Has(shaOf(keep)) {
		t.Error("pruned the wrong content")
	}
}

// A backup made before content was encrypted reads as it always did.
func TestPlainContentStillReads(t *testing.T) {
	dir := t.TempDir()
	data := []byte("from knot 0.41")
	putContent(dir, shaOf(data), string(data))
	c, err := NewContent(dir, testKey, false) // records encrypted, content not
	if err != nil {
		t.Fatal(err)
	}
	if got, err := readAll(t, c, shaOf(data)); err != nil || !bytes.Equal(got, data) {
		t.Errorf("plain content: %q %v", got, err)
	}
}

// Two contents never share a key and nonce: were they to, the two
// ciphertexts would differ exactly as the two contents do.
func TestEncryptedContentKeystreamsDiffer(t *testing.T) {
	c := sealed(t, t.TempDir(), testKey)
	a, b := bytes.Repeat([]byte{'a'}, 1000), bytes.Repeat([]byte{'b'}, 1000)
	c.Download(shaOf(a), serve(a))
	c.Download(shaOf(b), serve(b))
	sa, _ := os.ReadFile(c.path(shaOf(a)))
	sb, _ := os.ReadFile(c.path(shaOf(b)))
	same := 0
	for i := contentHeader; i < contentHeader+1000; i++ {
		if sa[i]^sb[i] == 'a'^'b' {
			same++
		}
	}
	if same > 50 {
		t.Errorf("%d of 1000 bytes give away that the contents share a keystream", same)
	}
}
