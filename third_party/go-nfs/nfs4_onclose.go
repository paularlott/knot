package nfs

import (
	"errors"
	"io"
	"os"
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
	// knot: a file closed is finished with: what was written goes to the
	// Committer now, without waiting for a COMMIT.
	// A file removed meanwhile has nothing left to save: its open is
	// closed all the same.
	if cm, ok := current.fs.(Committer); ok {
		if err := cm.Commit(current.fullPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nfs4StatusFromErr(err)
		}
	}
	id, status := c.w.Server.nfs4State().close(req.StateID)
	if status != nfs4OK {
		return status
	}
	return nfs4Encode(res, id)
}
