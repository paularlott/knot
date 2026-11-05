package nfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/willscott/go-nfs-client/nfs/xdr"
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

func TestHandlerPanicAnswered(t *testing.T) {
	c := &conn{Server: &Server{}}
	w := &response{req: &request{}}
	err := c.callHandler(context.Background(), w, func(context.Context, *response, Handler) error {
		panic("boom")
	})
	var sysErr *ResponseCodeSystemError
	if !errors.As(err, &sysErr) {
		t.Fatalf("panic answered with %v", err)
	}
}

// unreachableFS is a filesystem that cannot answer: its server is down.
type unreachableFS struct{ billy.Filesystem }

func (unreachableFS) Join(elem ...string) string { return strings.Join(elem, "/") }
func (unreachableFS) Stat(string) (os.FileInfo, error) {
	return nil, errors.New("dial tcp 127.0.0.1:8080: connect: connection refused")
}
func (f unreachableFS) Lstat(name string) (os.FileInfo, error) { return f.Stat(name) }

type unreachableHandler struct{ Handler }

func (unreachableHandler) FromHandle([]byte) (billy.Filesystem, []string, error) {
	return unreachableFS{}, []string{"f"}, nil
}

// A WRITE or SETATTR of a file that cannot be looked at for a moment is
// IO, not ACCES: told "permission denied", a program gives up for good.
func TestUnreachableFileIsNotPermissionDenied(t *testing.T) {
	write := func(v any) []byte {
		var b bytes.Buffer
		if err := xdr.Write(&b, v); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}
	for name, call := range map[string]struct {
		h    HandleFunc
		body []byte
	}{
		"WRITE":   {onWrite, write(writeArgs{Handle: []byte("h"), Count: 1, How: uint32(fileSync), Data: []byte("x")})},
		"SETATTR": {onSetAttr, append(write([]byte("h")), make([]byte, 64)...)},
	} {
		w := &response{req: &request{Body: bytes.NewReader(call.body)}}
		err := call.h(context.Background(), w, unreachableHandler{})
		var st *NFSStatusError
		if !errors.As(err, &st) || st.NFSStatus != NFSStatusIO {
			t.Errorf("%s of a file that cannot be looked at: %v", name, err)
		}
	}
}
