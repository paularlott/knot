package msg

// CopyFileMessage is the agent-side payload for a single file copy in either
// direction (to_space / from_space).
//
// Mode is the write semantics for to_space: "overwrite" (default), "append",
// or "prepend".
//
// MtimeNs and FilePerm are optional to_space metadata used by sync tools to
// preserve the source file's modification time and permission bits — both are
// applied AFTER the content write so the file ends up byte-identical to the
// source. Zero values mean "leave alone".
type CopyFileMessage struct {
	SourcePath    string `msgpack:"source_path"`
	DestPath      string `msgpack:"dest_path"`
	Content       []byte `msgpack:"content,omitempty"`
	Direction     string `msgpack:"direction"` // "to_space" or "from_space"
	Workdir       string `msgpack:"workdir"`
	Mode          string `msgpack:"mode,omitempty"`           // to_space only: "overwrite" (default), "append", "prepend"
	MtimeNs       int64  `msgpack:"mtime_ns,omitempty"`       // to_space only: unix nanoseconds; agent Chtimes after write
	FilePerm      uint32 `msgpack:"file_perm,omitempty"`      // to_space only: os.FileMode bits; agent Chmod after write
	SymlinkTarget string `msgpack:"symlink_target,omitempty"` // to_space only: when set, create symlink instead of writing content
	// from_space only: read ByteLength bytes from ByteOffset rather than the
	// whole file, so large files come back in pieces. ByteLength 0 reads
	// the whole file.
	ByteOffset int64 `msgpack:"byte_offset,omitempty"`
	ByteLength int64 `msgpack:"byte_length,omitempty"`
}

type CopyFileResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Content []byte `msgpack:"content,omitempty"`
	// Ranged answers a from_space read with a ByteLength: Content is that
	// range, and the file's size, mtime and permission bits come with it.
	Ranged   bool   `msgpack:"ranged,omitempty"`
	FileSize int64  `msgpack:"file_size,omitempty"`
	MtimeNs  int64  `msgpack:"mtime_ns,omitempty"`
	FilePerm uint32 `msgpack:"file_perm,omitempty"`
}
