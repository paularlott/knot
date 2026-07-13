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
type FindMessage struct {
	Path          string   `msgpack:"path" json:"path"`
	Recursive     bool     `msgpack:"recursive" json:"recursive"`
	Type          string   `msgpack:"type,omitempty" json:"type,omitempty"` // "any", "file", "dir"
	Name          string   `msgpack:"name,omitempty" json:"name,omitempty"`
	IncludeHidden bool     `msgpack:"include_hidden" json:"include_hidden"`
	FollowLinks   bool     `msgpack:"follow_links" json:"follow_links"`
	MaxDepth      int      `msgpack:"max_depth,omitempty" json:"max_depth,omitempty"`
	MtimeMin      *float64 `msgpack:"mtime_min,omitempty" json:"mtime_min,omitempty"`
	MtimeMax      *float64 `msgpack:"mtime_max,omitempty" json:"mtime_max,omitempty"`
	SizeMin       *int64   `msgpack:"size_min,omitempty" json:"size_min,omitempty"`
	SizeMax       *int64   `msgpack:"size_max,omitempty" json:"size_max,omitempty"`
	Workdir       string   `msgpack:"workdir,omitempty" json:"workdir,omitempty"`
}

type FindResponse struct {
	Success bool     `msgpack:"success" json:"success"`
	Error   string   `msgpack:"error,omitempty" json:"error,omitempty"`
	Paths   []string `msgpack:"paths,omitempty" json:"paths,omitempty"`
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
