package msg

// DeleteFileMessage removes a file or directory from the space's agent.
// Recursive is required to delete a non-empty directory (matches os.RemoveAll
// semantics); a non-recursive delete on a non-empty directory fails. Missing
// paths are treated as success so the call is idempotent.
type DeleteFileMessage struct {
	Path      string `msgpack:"path" json:"path"`
	Recursive bool   `msgpack:"recursive,omitempty" json:"recursive,omitempty"`
	Workdir   string `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

// DeleteFileResponse is the agent's reply to a DeleteFileMessage. Removed is
// the number of filesystem entries removed (1 for a single file; for a
// recursive delete it counts the tree). Missing paths return Success=true
// with Removed=0 so callers can be idempotent.
type DeleteFileResponse struct {
	Success bool   `msgpack:"success" json:"success"`
	Error   string `msgpack:"error,omitempty" json:"error,omitempty"`
	Removed int    `msgpack:"removed,omitempty" json:"removed,omitempty"`
}
