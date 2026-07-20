package msg

// GrepMessage searches file contents in the space's agent via the scriptling
// extlibs Grep function (parallel worker pool, no interpreter).
type GrepMessage struct {
	Pattern     string `msgpack:"pattern" json:"pattern"`
	Path        string `msgpack:"path" json:"path"`
	Literal     bool   `msgpack:"literal" json:"literal"`
	Recursive   bool   `msgpack:"recursive" json:"recursive"`
	IgnoreCase  bool   `msgpack:"ignore_case" json:"ignore_case"`
	Glob        string `msgpack:"glob,omitempty" json:"glob,omitempty"`
	FollowLinks bool   `msgpack:"follow_links" json:"follow_links"`
	MaxSize     int64  `msgpack:"max_size,omitempty" json:"max_size,omitempty"`
	Workdir     string `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

// GrepMatch is a single matching line.
type GrepMatch struct {
	File string `msgpack:"file" json:"file"`
	Line int    `msgpack:"line" json:"line"`
	Text string `msgpack:"text" json:"text"`
}

type GrepResponse struct {
	Success bool       `msgpack:"success" json:"success"`
	Error   string     `msgpack:"error,omitempty" json:"error,omitempty"`
	Matches []GrepMatch `msgpack:"matches,omitempty" json:"matches,omitempty"`
}

// FindMessage finds files/directories in the space's agent via the scriptling
// extlibs Find function (concurrent walker).
//
// By default the response carries only Paths (cheap — no per-entry stat when
// size/mtime filters are inactive). Set IncludeMetadata=true to get Entries
// with size, mtime, and is_dir per match — the agent stats every match in
// that mode, so only opt in when you actually need the metadata (e.g. for
// differential sync).
type FindMessage struct {
	Path            string   `msgpack:"path" json:"path"`
	Recursive       bool     `msgpack:"recursive" json:"recursive"`
	Type            string   `msgpack:"type,omitempty" json:"type,omitempty"` // "any", "file", "dir"
	Name            string   `msgpack:"name,omitempty" json:"name,omitempty"`
	IncludeHidden   bool     `msgpack:"include_hidden" json:"include_hidden"`
	IncludeMetadata bool     `msgpack:"include_metadata,omitempty" json:"include_metadata,omitempty"`
	IncludeHash     bool     `msgpack:"include_hash,omitempty" json:"include_hash,omitempty"`
	IncludeSymlinks bool     `msgpack:"include_symlinks,omitempty" json:"include_symlinks,omitempty"`
	FollowLinks     bool     `msgpack:"follow_links" json:"follow_links"`
	MaxDepth        int      `msgpack:"max_depth,omitempty" json:"max_depth,omitempty"`
	MtimeMin        *float64 `msgpack:"mtime_min,omitempty" json:"mtime_min,omitempty"`
	MtimeMax        *float64 `msgpack:"mtime_max,omitempty" json:"mtime_max,omitempty"`
	SizeMin         *int64   `msgpack:"size_min,omitempty" json:"size_min,omitempty"`
	SizeMax         *int64   `msgpack:"size_max,omitempty" json:"size_max,omitempty"`
	Workdir         string   `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

// FindEntry is a single matching file or directory, carrying the metadata
// callers need to decide whether the entry has changed without re-reading
// the bytes. Only populated when FindMessage.IncludeMetadata is set; the
// default response shape uses Paths instead (no per-entry stat cost).
// Hash is crc64-ISO of file content, populated when IncludeHash is set.
type FindEntry struct {
	Path       string  `msgpack:"path" json:"path"`
	Size       int64   `msgpack:"size" json:"size"`
	Mtime      float64 `msgpack:"mtime" json:"mtime"` // epoch seconds
	IsDir      bool    `msgpack:"is_dir" json:"is_dir"`
	Hash       uint64  `msgpack:"hash,omitempty" json:"hash,omitempty"`
	LinkTarget string  `msgpack:"link_target,omitempty" json:"link_target,omitempty"`
	FilePerm   int     `msgpack:"file_perm,omitempty" json:"file_perm,omitempty"`
}

// FindResponse is the agent's reply to a FindMessage.
//
// INVARIANT: exactly one of Paths or Entries is populated per response, never
// both, never neither (on success). The agent picks based on
// FindMessage.IncludeMetadata:
//
//   - IncludeMetadata=false (default) → Paths is populated. Cheap — the
//     underlying walker returns matching path strings without stat'ing each
//     entry (unless size/mtime filters force it).
//   - IncludeMetadata=true → Entries is populated. Every matching entry is
//     stat'd; only opt in when you actually need size/mtime/is_dir.
//
// Callers that read both fields are wrong; pick one based on the request flag.
type FindResponse struct {
	Success bool        `msgpack:"success" json:"success"`
	Error   string      `msgpack:"error,omitempty" json:"error,omitempty"`
	Paths   []string    `msgpack:"paths,omitempty" json:"paths,omitempty"`
	Entries []FindEntry `msgpack:"entries,omitempty" json:"entries,omitempty"`
}

// SedMessage performs an in-place edit or capture extraction in the space's
// agent via the scriptling extlibs Sed functions. Mode selects the operation:
// "replace" (literal), "replace_pattern" (regex), or "extract" (read-only
// capture-group extraction).
type SedMessage struct {
	Mode        string `msgpack:"mode" json:"mode"` // "replace", "replace_pattern", "extract"
	Pattern     string `msgpack:"pattern" json:"pattern"`
	Replacement string `msgpack:"replacement,omitempty" json:"replacement,omitempty"`
	Path        string `msgpack:"path" json:"path"`
	Recursive   bool   `msgpack:"recursive" json:"recursive"`
	IgnoreCase  bool   `msgpack:"ignore_case" json:"ignore_case"`
	Glob        string `msgpack:"glob,omitempty" json:"glob,omitempty"`
	FollowLinks bool   `msgpack:"follow_links" json:"follow_links"`
	MaxSize     int64  `msgpack:"max_size,omitempty" json:"max_size,omitempty"`
	Workdir     string `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

// ExtractMatch is a single match with capture groups (for extract mode).
type ExtractMatch struct {
	File   string   `msgpack:"file" json:"file"`
	Line   int      `msgpack:"line" json:"line"`
	Text   string   `msgpack:"text" json:"text"`
	Groups []string `msgpack:"groups,omitempty" json:"groups,omitempty"`
}

type SedResponse struct {
	Success       bool           `msgpack:"success" json:"success"`
	Error         string         `msgpack:"error,omitempty" json:"error,omitempty"`
	Mode          string         `msgpack:"mode,omitempty" json:"mode,omitempty"`
	FilesModified int64          `msgpack:"files_modified,omitempty" json:"files_modified,omitempty"` // replace / replace_pattern
	Matches       []ExtractMatch `msgpack:"matches,omitempty" json:"matches,omitempty"`        // extract
}

// EditFileMessage performs a targeted search-and-replace on a single file in
// the space's agent via the scriptling extlibs EditFile function. The search
// text must appear exactly once (uniqueness check); the modification is written
// atomically (temp file + rename).
type EditFileMessage struct {
	Path    string `msgpack:"path" json:"path"`
	Search  string `msgpack:"search" json:"search"`
	Replace string `msgpack:"replace" json:"replace"`
	Workdir string `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

type EditFileResponse struct {
	Success      bool   `msgpack:"success" json:"success"`
	Error        string `msgpack:"error,omitempty" json:"error,omitempty"`
	BytesWritten int    `msgpack:"bytes_written,omitempty" json:"bytes_written,omitempty"`
}
