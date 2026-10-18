package nfs

import (
	"bytes"
	"io"

	"github.com/willscott/go-nfs-client/nfs/xdr"
)

// nfs4ReadDirArgs is a READDIR4args. The cookie verifier is handled as the
// number getDirListingWithVerifier deals in.
type nfs4ReadDirArgs struct {
	Cookie      uint64
	CookieVerf  uint64
	DirCount    uint32
	MaxCount    uint32
	AttrRequest nfs4Bitmap
}

// nfs4DirEntry is an entry4 preceded by the flag saying it is present: the
// encoding of the list's *entry4 links.
type nfs4DirEntry struct {
	Follows bool
	Cookie  uint64
	Name    string
	Attrs   nfs4FAttr
}

type nfs4DirListEnd struct {
	Follows bool
	EOF     bool
}

func nfs4OnReadDir(c *nfs4Compound, args io.Reader, res io.Writer) nfs4Status {
	var req nfs4ReadDirArgs
	if status := nfs4Decode(args, &req); status != nfs4OK {
		return status
	}
	if req.MaxCount < 128 {
		return nfs4ErrTooSmall
	}
	current, status := c.requireCurrent()
	if status != nfs4OK {
		return status
	}
	if status := current.ensureDir(); status != nfs4OK {
		return status
	}
	// knot: cookies are a hash of the entry's name, in whose order the
	// listing is (see cookieFor), so a client resumes after the entry it
	// last had, however the folder changed meanwhile; no verifier is needed.
	contents, nfsErr := getDirListing(c.handler, current.handle)
	if nfsErr != nil {
		return nfs4StatusFromErr(nfsErr)
	}
	verifier := uint64(0)

	var entries bytes.Buffer
	eof := true
	for _, info := range contents {
		cookie := cookieFor(info.Name())
		if cookie <= req.Cookie {
			continue
		}
		entryPath := current.child(info.Name())
		entry := &nfs4FileHandle{handle: c.handler.ToHandle(current.fs, entryPath), fs: current.fs, path: entryPath}
		attrs, status := nfs4BuildAttrs(c.ctx, c.handler, entry, info, req.AttrRequest)
		if status != nfs4OK {
			return status
		}

		var one bytes.Buffer
		if err := xdr.Write(&one, nfs4DirEntry{Follows: true, Cookie: cookie, Name: info.Name(), Attrs: attrs}); err != nil {
			return nfs4ErrServerFault
		}
		// 16 bytes for the verifier and the list's end.
		if entries.Len()+one.Len()+16 > int(req.MaxCount) {
			if entries.Len() == 0 {
				return nfs4ErrTooSmall
			}
			eof = false
			break
		}
		entries.Write(one.Bytes())
	}

	if status := nfs4Encode(res, verifier); status != nfs4OK {
		return status
	}
	if _, err := res.Write(entries.Bytes()); err != nil {
		return nfs4ErrServerFault
	}
	return nfs4Encode(res, nfs4DirListEnd{Follows: false, EOF: eof})
}
