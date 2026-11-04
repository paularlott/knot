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
	return fileStamp{ino: st.Ino, ctime: st.Ctimespec.Sec*1e9 + st.Ctimespec.Nsec}
}
