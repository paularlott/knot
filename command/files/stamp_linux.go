package command_files

import (
	"os"
	"syscall"
)

func platformStamp(info os.FileInfo) fileStamp {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fileStamp{}
	}
	return fileStamp{ino: st.Ino, ctime: int64(st.Ctim.Sec)*1e9 + int64(st.Ctim.Nsec)}
}
