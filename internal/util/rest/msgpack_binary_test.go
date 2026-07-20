package rest

import (
	"bytes"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
)

// TestMsgpackBinaryContentRoundTrip proves msgpack's str type preserves
// arbitrary bytes — including invalid UTF-8 — when round-tripping through a
// Go string field. This is the property mirror relies on for binary files
// (.DS_Store, .vlog, image assets, etc.). If this test fails, the
// "invalid msgpack format" / re-upload-every-time bug is in the encoder;
// if it passes, the bug is elsewhere (truncation, mtime drift, etc.).
func TestMsgpackBinaryContentRoundTrip(t *testing.T) {
	type request struct {
		Path    string `msgpack:"path"`
		Content string `msgpack:"content"`
	}

	// Build content with every byte 0..255 — guarantees invalid UTF-8 sequences.
	allBytes := make([]byte, 256)
	for i := range allBytes {
		allBytes[i] = byte(i)
	}

	original := request{Path: "/x", Content: string(allBytes)}
	encoded, err := msgpack.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded request
	if err := msgpack.NewDecoder(bytes.NewReader(encoded)).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if decoded.Path != "/x" {
		t.Errorf("path: got %q, want /x", decoded.Path)
	}
	got := []byte(decoded.Content)
	if !bytes.Equal(got, allBytes) {
		t.Errorf("content bytes differ: got len=%d, want len=%d", len(got), len(allBytes))
		// Find first divergence for debugging.
		for i := range allBytes {
			if got[i] != allBytes[i] {
				t.Errorf("first diff at byte %d: got %x, want %x", i, got[i], allBytes[i])
				break
			}
		}
	}
}

// TestMsgpackLargeContentRoundTrip covers the badgerdb .vlog case
// specifically: a large payload (1 MiB of random-ish binary). If msgpack
// can't handle this, large file uploads are doomed regardless of transport.
func TestMsgpackLargeContentRoundTrip(t *testing.T) {
	type request struct {
		Path    string `msgpack:"path"`
		Content string `msgpack:"content"`
	}

	// 1 MiB of pseudo-random bytes (deterministic — same seed every run).
	rng := bytes.NewBuffer(nil)
	seed := byte(0x42)
	for i := 0; i < 1024*1024; i++ {
		seed = seed*31 + 7
		rng.WriteByte(seed)
	}
	payload := rng.Bytes()

	original := request{Path: "/big.bin", Content: string(payload)}
	encoded, err := msgpack.Marshal(&original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded request
	if err := msgpack.NewDecoder(bytes.NewReader(encoded)).Decode(&decoded); err != nil {
		t.Fatalf("decode: %v", err)
	}

	got := []byte(decoded.Content)
	if !bytes.Equal(got, payload) {
		t.Errorf("content bytes differ: got len=%d, want len=%d", len(got), len(payload))
	}
}
