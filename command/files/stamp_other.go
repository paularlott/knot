//go:build !darwin && !linux

package command_files

import "os"

// Elsewhere only size and modification time are compared.
func platformStamp(os.FileInfo) fileStamp { return fileStamp{} }
