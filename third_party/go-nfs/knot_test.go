package nfs

import (
	"bytes"
	"encoding/binary"
	"os"
	"syscall"
	"testing"
)

func TestReadOpaqueSkipsPadding(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(1)) // a one byte handle
	b.Write([]byte{0xAB, 0, 0, 0})                // and its padding
	binary.Write(&b, binary.BigEndian, uint32(0x3f))

	got, err := readOpaque(&b)
	if err != nil || !bytes.Equal(got, []byte{0xAB}) {
		t.Fatalf("opaque = %x %v", got, err)
	}
	var next uint32
	binary.Read(&b, binary.BigEndian, &next)
	if next != 0x3f {
		t.Fatalf("argument after the opaque = %#x", next)
	}
}

func TestReadOpaqueBounded(t *testing.T) {
	var b bytes.Buffer
	binary.Write(&b, binary.BigEndian, uint32(maxOpaque+1))
	if _, err := readOpaque(&b); err == nil {
		t.Fatal("oversized opaque accepted")
	}
}

func TestStatusFromError(t *testing.T) {
	for err, want := range map[error]NFSStatus{
		&os.PathError{Op: "remove", Err: syscall.ENOTEMPTY}: NFSStatusNotEmpty,
		&os.LinkError{Op: "rename", Err: syscall.EXDEV}:     NFSStatusXDev,
		&os.PathError{Op: "open", Err: os.ErrNotExist}:      NFSStatusNoEnt,
		&os.PathError{Op: "open", Err: os.ErrExist}:         NFSStatusExist,
		&os.PathError{Op: "write", Err: syscall.EDQUOT}:     NFSStatusDQuot,
		&NFSStatusError{NFSStatus: NFSStatusStale}:          NFSStatusStale,
		os.ErrClosed: NFSStatusIO,
	} {
		if got := statusFromError(err, NFSStatusIO); got != want {
			t.Errorf("%v: got %v, want %v", err, got, want)
		}
	}
}

func TestCookiesAreStable(t *testing.T) {
	// A name's cookie does not depend on the other entries, and never
	// collides with "." (0) or ".." (1).
	for _, n := range []string{"a", "b", "1000", ""} {
		if c := cookieFor(n); c <= 1 || c != cookieFor(n) {
			t.Fatalf("cookie for %q = %d", n, c)
		}
	}
}
