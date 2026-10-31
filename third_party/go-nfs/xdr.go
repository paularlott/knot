// knot: this file is knot's; see README.knot.md.

package nfs

import (
	"errors"
	"io"

	"github.com/willscott/go-nfs-client/nfs/xdr"
)

// maxOpaque bounds the variable-length opaques read from a request: file
// handles (64 bytes), paths and link targets.
const maxOpaque = 64 << 10

// readOpaque reads an XDR variable-length opaque.
//
// knot: xdr.ReadOpaque, used upstream, does not skip the padding after data
// that is not a multiple of 4 bytes long, so the arguments after an unaligned
// file handle were read from the padding (ACCESS read a mask of 0 and denied
// everything). It could also return short, and allocated whatever length the
// client sent.
func readOpaque(r io.Reader) ([]byte, error) {
	length, err := xdr.ReadUint32(r)
	if err != nil {
		return nil, err
	}
	if length > maxOpaque {
		return nil, errors.New("xdr opaque too long")
	}
	buf := make([]byte, length+(4-length%4)%4)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf[:length], nil
}
