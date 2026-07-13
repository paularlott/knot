package msg

type CopyFileMessage struct {
	SourcePath string `msgpack:"source_path"`
	DestPath   string `msgpack:"dest_path"`
	Content    []byte `msgpack:"content,omitempty"`
	Direction  string `msgpack:"direction"` // "to_space" or "from_space"
	Workdir    string `msgpack:"workdir"`
	Mode       string `msgpack:"mode,omitempty"` // to_space only: "overwrite" (default), "append", "prepend"
}

type CopyFileResponse struct {
	Success bool   `msgpack:"success"`
	Error   string `msgpack:"error"`
	Content []byte `msgpack:"content,omitempty"`
}
