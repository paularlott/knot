package nfs

import (
	"io"
)

type nfs4CloseArgs struct {
	Seqid   uint32
	StateID nfs4StateID
}

func nfs4OnClose(c *nfs4Compound, args io.Reader, res io.Writer) nfs4Status {
	var req nfs4CloseArgs
	if status := nfs4Decode(args, &req); status != nfs4OK {
		return status
	}
	current, status := c.requireCurrentOrGhost()
	if status != nfs4OK {
		return status
	}
	if current.ghost {
		// knot: the open of a file removed meanwhile is closed by its
		// stateid alone (see nfs4OnPutFH). One already gone answers
		// NFS4ERR_NOENT, which the Mac's client takes as closed, where
		// NFS4ERR_BAD_STATEID would send it reclaiming its opens.
		id, status := c.w.Server.nfs4State().close(req.StateID)
		if status != nfs4OK {
			return nfs4ErrNoEnt
		}
		c.ghostClosed = true
		return nfs4Encode(res, id)
	}
	// knot: CLOSE saves nothing. A client saves what it wrote before the
	// program's close() returns, with FILE_SYNC writes or a COMMIT, and the
	// Committer saves anything else once writes stop. Clients send CLOSE
	// later, on their own time: Linux after close() has returned, the Mac
	// when it lets go of the file. By then the handle can name another
	// file's content: a file written, closed and renamed over another
	// has its CLOSE arrive after the rename, and after the program has
	// opened the name again and begun writing it afresh, and saving at that
	// CLOSE saved that half-written file for everyone to read. A failed
	// CLOSE, the Mac's client sends again at once, forever (NFS.kext
	// nfs_vnop_inactive), so it never fails for a save.
	id, status := c.w.Server.nfs4State().close(req.StateID)
	if status != nfs4OK {
		return status
	}
	return nfs4Encode(res, id)
}
