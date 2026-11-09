package nfs

import "io"

type nfs4PutFHArgs struct {
	Handle []byte
}

func nfs4OnPutFH(c *nfs4Compound, args io.Reader, _ io.Writer) nfs4Status {
	var req nfs4PutFHArgs
	if status := nfs4Decode(args, &req); status != nfs4OK {
		return status
	}
	if len(req.Handle) > nfs4FhSize {
		return nfs4ErrBadHandle
	}
	fs, p, err := c.handler.FromHandle(req.Handle)
	if err != nil {
		// knot: a handle that no longer resolves, its file removed, is a
		// ghost: PUTFH succeeds, and the operations that need the file
		// answer NFS4ERR_STALE as PUTFH did, but CLOSE and LOCKU, which go
		// by stateid, finish the open and the locks the client still has.
		// The Mac's client, letting go of a file it held open when it was
		// removed, sends PUTFH CLOSE GETATTR and, on any error but
		// NFS4ERR_NOENT, sends it again at once, forever (NFS.kext,
		// nfs_vnop_inactive): with every other process using that file's
		// folder waiting behind it.
		c.current = &nfs4FileHandle{handle: append([]byte(nil), req.Handle...), ghost: true}
		return nfs4OK
	}
	c.current = &nfs4FileHandle{handle: req.Handle, fs: fs, path: nfs4CopyPath(p)}
	return nfs4OK
}
