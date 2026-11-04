package command_files

import "os"

// fileStamp is what the file system says of a file beyond its size and
// modification time: its inode number and the time its inode last changed.
// A tool that restores modification times (cp -p, tar x, rsync -t, touch -r)
// can change a file's content and leave its size and time as they were, but
// not its change time, which the system sets on every write and cannot be set
// back; a file replaced by another, renamed over it, has another inode. So a
// file whose stamp differs is read again rather than taken to be unchanged.
// Both are zero where the platform does not give them, and then only size and
// time are compared, as before.
type fileStamp struct {
	ino   uint64
	ctime int64 // unix nanoseconds
}

func stampOf(info os.FileInfo) fileStamp {
	if info == nil {
		return fileStamp{}
	}
	return platformStamp(info)
}

// stampMatches reports whether a stamp recorded when a file was last read
// still holds: always where the platform gives none, else only when equal,
// so a record made without one (by an earlier knot) has the file read again
// once.
func stampMatches(recorded, now fileStamp) bool {
	return now == (fileStamp{}) || recorded == now
}
