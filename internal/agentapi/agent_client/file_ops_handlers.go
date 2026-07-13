package agent_client

import (
	"context"
	"net"
	"path/filepath"

	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/scriptling/extlibs"
)

// resolvePath joins path with workdir when path is relative and workdir is set.
func resolvePath(path, workdir string) string {
	if path == "" || workdir == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workdir, path)
}

func sendResponse(stream net.Conn, resp interface{}) {
	if err := msg.WriteMessage(stream, resp); err != nil {
		log.WithError(err).Error("failed to send file-ops response")
	}
}

// --- Grep --------------------------------------------------------------------

func handleGrepExecution(stream net.Conn, g msg.GrepMessage) {
	resp := msg.GrepResponse{}
	defer sendResponse(stream, &resp)

	matches, err := extlibs.Grep(context.Background(), g.Pattern, resolvePath(g.Path, g.Workdir), extlibs.GrepOptions{
		Literal:     g.Literal,
		Recursive:   g.Recursive,
		IgnoreCase:  g.IgnoreCase,
		FollowLinks: g.FollowLinks,
		Glob:        g.Glob,
		MaxSize:     g.MaxSize,
	})
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Matches = make([]msg.GrepMatch, len(matches))
	for i, m := range matches {
		resp.Matches[i] = msg.GrepMatch{File: m.File, Line: m.Line, Text: m.Text}
	}
	resp.Success = true
}

// --- Find --------------------------------------------------------------------

func handleFindExecution(stream net.Conn, f msg.FindMessage) {
	resp := msg.FindResponse{}
	defer sendResponse(stream, &resp)

	opts := extlibs.FindOptions{
		Type:          f.Type,
		Name:          f.Name,
		IncludeHidden: f.IncludeHidden,
		FollowLinks:   f.FollowLinks,
		MaxDepth:      f.MaxDepth,
		MtimeMin:      f.MtimeMin,
		MtimeMax:      f.MtimeMax,
		SizeMin:       f.SizeMin,
		SizeMax:       f.SizeMax,
	}
	// Find defaults recursive=true; only send when explicitly set.
	rec := f.Recursive
	opts.Recursive = &rec

	paths, err := extlibs.Find(context.Background(), resolvePath(f.Path, f.Workdir), opts)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.Paths = paths
	resp.Success = true
}

// --- Sed ---------------------------------------------------------------------

func handleSedExecution(stream net.Conn, s msg.SedMessage) {
	resp := msg.SedResponse{Mode: s.Mode}
	defer sendResponse(stream, &resp)

	path := resolvePath(s.Path, s.Workdir)
	opts := extlibs.SedOptions{
		Recursive:   s.Recursive,
		IgnoreCase:  s.IgnoreCase,
		FollowLinks: s.FollowLinks,
		Glob:        s.Glob,
		MaxSize:     s.MaxSize,
	}

	switch s.Mode {
	case "replace":
		n, err := extlibs.SedReplace(context.Background(), s.Pattern, s.Replacement, path, opts)
		if err != nil {
			resp.Error = err.Error()
			return
		}
		resp.FilesModified = n

	case "replace_pattern":
		n, err := extlibs.SedReplacePattern(context.Background(), s.Pattern, s.Replacement, path, opts)
		if err != nil {
			resp.Error = err.Error()
			return
		}
		resp.FilesModified = n

	case "extract":
		matches, err := extlibs.SedExtract(context.Background(), s.Pattern, path, opts)
		if err != nil {
			resp.Error = err.Error()
			return
		}
		resp.Matches = make([]msg.ExtractMatch, len(matches))
		for i, m := range matches {
			groups := make([]string, len(m.Groups))
			copy(groups, m.Groups)
			resp.Matches[i] = msg.ExtractMatch{File: m.File, Line: m.Line, Text: m.Text, Groups: groups}
		}

	default:
		resp.Error = "unknown sed mode: " + s.Mode
		return
	}

	resp.Success = true
}

// --- EditFile ----------------------------------------------------------------

func handleEditFileExecution(stream net.Conn, e msg.EditFileMessage) {
	resp := msg.EditFileResponse{}
	defer sendResponse(stream, &resp)

	n, err := extlibs.EditFile(context.Background(), resolvePath(e.Path, e.Workdir), e.Search, e.Replace)
	if err != nil {
		resp.Error = err.Error()
		return
	}
	resp.BytesWritten = n
	resp.Success = true
}
