package nfs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path"
	"sort"

	"github.com/willscott/go-nfs-client/nfs/xdr"
)

type readDirArgs struct {
	Handle      []byte
	Cookie      uint64
	CookieVerif uint64
	Count       uint32
}

type readDirEntity struct {
	FileID uint64
	Name   []byte
	Cookie uint64
	Next   bool
}

func onReadDir(ctx context.Context, w *response, userHandle Handler) error {
	w.errorFmt = opAttrErrorFormatter
	obj := readDirArgs{}
	err := xdr.Read(w.req.Body, &obj)
	if err != nil {
		return &NFSStatusError{NFSStatusInval, err}
	}

	if obj.Count < 1024 {
		return &NFSStatusError{NFSStatusTooSmall, io.ErrShortBuffer}
	}

	fs, p, err := userHandle.FromHandle(obj.Handle)
	if err != nil {
		return &NFSStatusError{NFSStatusStale, err}
	}

	// knot: cookies are stable (see cookieFor), so no verifier is needed
	// and a listing is always read afresh.
	contents, err := getDirListing(userHandle, obj.Handle)
	if err != nil {
		return err
	}
	verifier := uint64(0)

	entities := make([]readDirEntity, 0)
	maxBytes := uint32(100) // conservative overhead measure

	started := obj.Cookie == 0
	if started {
		// add '.' and '..' to entities
		dotdotFileID := uint64(0)
		if len(p) > 0 {
			dda := tryStat(fs, p[0:len(p)-1])
			if dda != nil {
				dotdotFileID = dda.Fileid
			}
		}
		dotFileID := uint64(0)
		da := tryStat(fs, p)
		if da != nil {
			dotFileID = da.Fileid
		}
		entities = append(entities,
			readDirEntity{Name: []byte("."), Cookie: 0, Next: true, FileID: dotFileID},
			readDirEntity{Name: []byte(".."), Cookie: 1, Next: true, FileID: dotdotFileID},
		)
	}

	eof := true
	maxEntities := userHandle.HandleLimit() / 2
	for _, c := range contents {
		// knot: resume after the client's cookie, not at an index, so
		// entries removed meanwhile do not make it skip others.
		cookie := cookieFor(c.Name())
		if cookie > obj.Cookie {
			maxBytes += 512 // TODO: better estimation.
			if maxBytes > obj.Count || len(entities) > maxEntities {
				eof = false
				break
			}

			attrs := ToFileAttribute(c, path.Join(append(p, c.Name())...))
			entities = append(entities, readDirEntity{
				FileID: attrs.Fileid,
				Name:   []byte(c.Name()),
				Cookie: cookie,
				Next:   true,
			})
		}
	}

	writer := bytes.NewBuffer([]byte{})
	if err := xdr.Write(writer, uint32(NFSStatusOk)); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	if err := WritePostOpAttrs(writer, tryStat(fs, p)); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}

	if err := xdr.Write(writer, verifier); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}

	if err := xdr.Write(writer, len(entities) > 0); err != nil { // next
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	if len(entities) > 0 {
		entities[len(entities)-1].Next = false
		// no next for last entity

		for _, e := range entities {
			if err := xdr.Write(writer, e); err != nil {
				return &NFSStatusError{NFSStatusServerFault, err}
			}
		}
	}
	if err := xdr.Write(writer, eof); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	// TODO: track writer size at this point to validate maxcount estimation and stop early if needed.

	if err := w.Write(writer.Bytes()); err != nil {
		return &NFSStatusError{NFSStatusServerFault, err}
	}
	return nil
}

// getDirListing reads a directory in cookie order.
//
// knot: added, in place of getDirListingWithVerifier for READDIR and
// READDIRPLUS (NFSv3 and NFSv4).
func getDirListing(userHandle Handler, fsHandle []byte) ([]fs.FileInfo, error) {
	fs, p, err := userHandle.FromHandle(fsHandle)
	if err != nil {
		return nil, &NFSStatusError{NFSStatusStale, err}
	}
	contents, err := fs.ReadDir(fs.Join(p...))
	if err != nil {
		if os.IsPermission(err) {
			return nil, &NFSStatusError{NFSStatusAccess, err}
		}
		return nil, &NFSStatusError{NFSStatusNotDir, err}
	}
	sort.Slice(contents, func(i, j int) bool {
		ci, cj := cookieFor(contents[i].Name()), cookieFor(contents[j].Name())
		if ci != cj {
			return ci < cj
		}
		return contents[i].Name() < contents[j].Name()
	})
	return contents, nil
}

// cookieFor is the READDIR cookie of an entry: a hash of its name, so it
// stays valid while other entries come and go. 0 starts a listing, and 1 and
// 2 are "." and ".." (NFSv3) or reserved (NFSv4).
//
// knot: upstream used the entry's index in the listing, which a Linux client
// deleting entries between calls (rm -rf) resumes at the wrong place.
func cookieFor(name string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(name))
	if c := h.Sum64(); c > 2 {
		return c
	}
	return 3
}

func getDirListingWithVerifier(userHandle Handler, fsHandle []byte, verifier uint64) ([]fs.FileInfo, uint64, error) {
	// figure out what directory it is.
	fs, p, err := userHandle.FromHandle(fsHandle)
	if err != nil {
		return nil, 0, &NFSStatusError{NFSStatusStale, err}
	}

	path := fs.Join(p...)
	// see if the verifier has this dir cached:
	if vh, ok := userHandle.(CachingHandler); verifier != 0 && ok {
		entries := vh.DataForVerifier(path, verifier)
		if entries != nil {
			return entries, verifier, nil
		}
	}
	// load the entries.
	contents, err := fs.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil, 0, &NFSStatusError{NFSStatusAccess, err}
		}
		return nil, 0, &NFSStatusError{NFSStatusNotDir, err}
	}

	sort.Slice(contents, func(i, j int) bool {
		return contents[i].Name() < contents[j].Name()
	})

	if vh, ok := userHandle.(CachingHandler); ok {
		// let the user handler make a verifier if it can.
		v := vh.VerifierFor(path, contents)
		return contents, v, nil
	}

	id := hashPathAndContents(path, contents)
	return contents, id, nil
}

func hashPathAndContents(path string, contents []fs.FileInfo) uint64 {
	//calculate a cookie-verifier.
	vHash := sha256.New()

	// Add the path to avoid collisions of directories with the same content
	vHash.Write([]byte(path))

	for _, c := range contents {
		vHash.Write([]byte(c.Name())) // Never fails according to the docs
	}

	verify := vHash.Sum(nil)[0:8]
	return binary.BigEndian.Uint64(verify)
}
