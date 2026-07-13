package msg

// GrepMessage searches file contents in the space's agent via the scriptling
// extlibs Grep function (parallel worker pool, no interpreter).
type GrepMessage struct {
	Pattern     string `msgpack:"pattern"`
	Path        string `msgpack:"path"`
	Literal     bool   `msgpack:"literal"`
	Recursive   bool   `msgpack:"recursive"`
	IgnoreCase  bool   `msgpack:"ignore_case"`
	Glob        string `msgpack:"glob,omitempty"`
	FollowLinks bool   `msgpack:"follow_links"`
	MaxSize     int64  `msgpack:"max_size,omitempty"`
	Workdir     string `msgpack:"workdir,omitempty"`
}

// GrepMatch is a single matching line.
type GrepMatch struct {
	File string `msgpack:"file"`
	Line int    `msgpack:"line"`
	Text string `msgpack:"text"`
}

type GrepResponse struct {
	Success bool       `msgpack:"success"`
	Error   string     `msgpack:"error,omitempty"`
	Matches []GrepMatch `msgpack:"matches,omitempty"`
}

// FindMessage finds files/directories in the space's agent via the scriptling
// extlibs Find function (concurrent walker).
type FindMessage struct {
	Path          string   `msgpack:"path"`
	Recursive     bool     `msgpack:"recursive"`
	Type          string   `msgpack:"type,omitempty"` // "any", "file", "dir"
	Name          string   `msgpack:"name,omitempty"`
	IncludeHidden bool     `msgpack:"include_hidden"`
	FollowLinks   bool     `msgpack:"follow_links"`
	MaxDepth      int      `msgpack:"max_depth,omitempty"`
	MtimeMin      *float64 `msgpack:"mtime_min,omitempty"`
	MtimeMax      *float64 `msgpack:"mtime_max,omitempty"`
	SizeMin       *int64   `msgpack:"size_min,omitempty"`
	SizeMax       *int64   `msgpack:"size_max,omitempty"`
	Workdir       string   `msgpack:"workdir,omitempty"`
}

type FindResponse struct {
	Success bool     `msgpack:"success"`
	Error   string   `msgpack:"error,omitempty"`
	Paths   []string `msgpack:"paths,omitempty"`
}

// SedMessage performs an in-place edit or capture extraction in the space's
// agent via the scriptling extlibs Sed functions. Mode selects the operation:
// "replace" (literal), "replace_pattern" (regex), or "extract" (read-only
// capture-group extraction).
type SedMessage struct {
	Mode        string `msgpack:"mode"` // "replace", "replace_pattern", "extract"
	Pattern     string `msgpack:"pattern"`
	Replacement string `msgpack:"replacement,omitempty"`
	Path        string `msgpack:"path"`
	Recursive   bool   `msgpack:"recursive"`
	IgnoreCase  bool   `msgpack:"ignore_case"`
	Glob        string `msgpack:"glob,omitempty"`
	FollowLinks bool   `msgpack:"follow_links"`
	MaxSize     int64  `msgpack:"max_size,omitempty"`
	Workdir     string `msgpack:"workdir,omitempty"`
}

// ExtractMatch is a single match with capture groups (for extract mode).
type ExtractMatch struct {
	File   string   `msgpack:"file"`
	Line   int      `msgpack:"line"`
	Text   string   `msgpack:"text"`
	Groups []string `msgpack:"groups,omitempty"`
}

type SedResponse struct {
	Success       bool           `msgpack:"success"`
	Error         string         `msgpack:"error,omitempty"`
	Mode          string         `msgpack:"mode,omitempty"`
	FilesModified int64          `msgpack:"files_modified,omitempty"` // replace / replace_pattern
	Matches       []ExtractMatch `msgpack:"matches,omitempty"`        // extract
}

// EditFileMessage performs a targeted search-and-replace on a single file in
// the space's agent via the scriptling extlibs EditFile function. The search
// text must appear exactly once (uniqueness check); the modification is written
// atomically (temp file + rename).
type EditFileMessage struct {
	Path    string `msgpack:"path"`
	Search  string `msgpack:"search"`
	Replace string `msgpack:"replace"`
	Workdir string `msgpack:"workdir,omitempty"`
}

type EditFileResponse struct {
	Success      bool   `msgpack:"success"`
	Error        string `msgpack:"error,omitempty"`
	BytesWritten int    `msgpack:"bytes_written,omitempty"`
}
