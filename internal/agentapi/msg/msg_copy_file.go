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
	Mode          string `msgpack:"mode,omitempty"`     // to_space only: "overwrite" (default), "append", "prepend"
	MtimeNs       int64  `msgpack:"mtime_ns,omitempty"` // to_space only: unix nanoseconds; agent Chtimes after write
	FilePerm      uint32 `msgpack:"file_perm,omitempty"` // to_space only: os.FileMode bits; agent Chmod after write
	SymlinkTarget string `msgpack:"symlink_target,omitempty"` // to_space only: when set, create symlink instead of writing content
}

type CopyFileResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Content []byte `msgpack:"content,omitempty"`
}
