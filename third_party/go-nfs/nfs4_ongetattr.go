package nfs

import "io"

type nfs4GetAttrArgs struct {
	Request nfs4Bitmap
}

func nfs4OnGetAttr(c *nfs4Compound, args io.Reader, res io.Writer) nfs4Status {
	var req nfs4GetAttrArgs
	if status := nfs4Decode(args, &req); status != nfs4OK {
		return status
	}
	if c.ghostClosed && c.current != nil && c.current.ghost {
		// knot: the GETATTR after closing a removed file's open (see
		// nfs4OnPutFH): NFS4ERR_NOENT, the one error after which the Mac's
		// client lets the file go rather than close it again.
		return nfs4ErrNoEnt
	}
	current, status := c.requireCurrent()
	if status != nfs4OK {
		return status
	}
	info, err := current.fs.Lstat(current.fullPath())
	if err != nil {
		return nfs4FileStatusFromErr(err)
	}
	attrs, status := nfs4BuildAttrs(c.ctx, c.handler, current, info, req.Request)
	if status != nfs4OK {
		return status
	}
	return nfs4Encode(res, attrs)
}
