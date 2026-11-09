package nfs

import (
	"errors"
	"io"
	"os"
)

type nfs4ReadArgs struct {
	StateID nfs4StateID
	Offset  uint64
	Count   uint32
}

type nfs4ReadRes struct {
	EOF  bool
	Data []byte
}

func nfs4OnRead(c *nfs4Compound, args io.Reader, res io.Writer) nfs4Status {
	var req nfs4ReadArgs
	if status := nfs4Decode(args, &req); status != nfs4OK {
		return status
	}
	current, status := c.requireCurrent()
	if status != nfs4OK {
		return status
	}
	if status := c.w.Server.nfs4State().checkIO(req.StateID); status != nfs4OK {
		return status
	}
	if req.Count > nfs4MaxRead {
		req.Count = nfs4MaxRead
	}
	file, err := current.fs.Open(current.fullPath())
	if err != nil {
		return nfs4FileStatusFromErr(err)
	}
	defer file.Close()

	data := make([]byte, req.Count)
	var n int
	var info os.FileInfo
	if vr, ok := file.(VersionedReader); ok {
		// knot: the end of file is that of the version read.
		n, info, err = vr.ReadAtInfo(data, int64(req.Offset))
	} else {
		n, err = file.ReadAt(data, int64(req.Offset))
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return nfs4FileStatusFromErr(err)
	}
	eof := errors.Is(err, io.EOF)
	if !eof {
		if info == nil {
			info, _ = current.fs.Stat(current.fullPath())
		}
		if info != nil {
			eof = int64(req.Offset)+int64(n) >= info.Size()
		}
	}
	return nfs4Encode(res, nfs4ReadRes{EOF: eof, Data: data[:n]})
}
