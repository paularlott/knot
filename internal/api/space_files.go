package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/paularlott/knot/internal/agentapi/agent_server"
	"github.com/paularlott/knot/internal/agentapi/msg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util/rest"
	"github.com/paularlott/knot/internal/util/validate"
)

type ReadFileRequest struct {
	Path   string `json:"path" msgpack:"path"`
	Offset int    `json:"offset,omitempty" msgpack:"offset,omitempty"` // 1-based line number to start at (0/absent = from beginning)
	Limit  int    `json:"limit,omitempty" msgpack:"limit,omitempty"`   // max lines to return (0/absent = no limit)
}

type ReadFileResponse struct {
	Success    bool   `json:"success" msgpack:"success"`
	Content    string `json:"content,omitempty" msgpack:"content,omitempty"`
	Size       int    `json:"size,omitempty" msgpack:"size,omitempty"`
	TotalLines int    `json:"total_lines,omitempty" msgpack:"total_lines,omitempty"` // total lines in the file (only when offset/limit used)
	Offset     int    `json:"offset,omitempty" msgpack:"offset,omitempty"`           // applied offset (1-based, only when offset/limit used)
	Limit      int    `json:"limit,omitempty" msgpack:"limit,omitempty"`             // applied limit (only when offset/limit used)
	Error      string `json:"error,omitempty" msgpack:"error,omitempty"`
}

type WriteFileRequest struct {
	Path    string `json:"path" msgpack:"path"`
	Content string `json:"content" msgpack:"content"`
	Mode    string `json:"mode,omitempty" msgpack:"mode,omitempty"`

	// SymlinkTarget, when set, creates a symlink at Path pointing to this
	// target instead of writing content. Used by knot mirror for symlinks.
	SymlinkTarget string `json:"symlink_target,omitempty" msgpack:"symlink_target,omitempty"`

	// Sync metadata (optional). When set, the agent applies them after the
	// write so the destination file matches the source's mtime and permission
	// bits — used by knot sync tools (copy --recursive, watch).
	MtimeNs  int64  `json:"mtime_ns,omitempty" msgpack:"mtime_ns,omitempty"`
	FilePerm uint32 `json:"file_perm,omitempty" msgpack:"file_perm,omitempty"`
}

type WriteFileResponse struct {
	Success      bool   `json:"success" msgpack:"success"`
	BytesWritten int    `json:"bytes_written,omitempty" msgpack:"bytes_written,omitempty"`
	Error        string `json:"error,omitempty" msgpack:"error,omitempty"`
}

func HandleReadSpaceFile(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	var req ReadFileRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Path == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "path is required"})
		return
	}

	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}
	spaceId = space.Id

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to get template"})
		return
	}

	if !template.WithRunCommand {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "File operations are not allowed in this space"})
		return
	}

	if space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to read files in this space"})
		return
	}

	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return
	}

	copyCmd := &msg.CopyFileMessage{
		SourcePath: req.Path,
		Direction:  "from_space",
		Workdir:    "",
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send read file command to agent: %v", err)})
		return
	}

	response := <-responseChannel
	if response == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}

	result := ReadFileResponse{
		Success: response.Success,
	}

	if !response.Success {
		result.Error = response.Error
	} else {
		content := string(response.Content)
		// Apply 1-based line offset/limit when requested. Offset/limit of 0
		// (or absent) means "not set", preserving the whole-file default.
		if req.Offset > 0 || req.Limit > 0 {
			content, result.TotalLines = sliceLines(content, req.Offset, req.Limit)
			result.Offset = req.Offset
			result.Limit = req.Limit
		}
		result.Content = content
		result.Size = len(content)
	}

	rest.WriteResponse(http.StatusOK, w, r, result)
}

// sliceLines extracts a 1-based line range from content. offset is 1-based
// (offset=1 or 0 = start at first line); limit is the max number of lines
// (0 = no limit). The returned totalLines is the file's total line count
// (excluding the trailing-newline artifact) so callers can page through.
func sliceLines(content string, offset, limit int) (sliced string, totalLines int) {
	lines := strings.Split(content, "\n")
	// Drop the trailing "" that Split produces when the file ends with "\n"
	// so the line count matches what an editor shows.
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	totalLines = len(lines)

	start := 0
	if offset > 1 {
		start = offset - 1
		if start > totalLines {
			start = totalLines
		}
	}
	end := totalLines
	if limit > 0 {
		end = start + limit
		if end > totalLines {
			end = totalLines
		}
	}
	return strings.Join(lines[start:end], "\n"), totalLines
}

func HandleWriteSpaceFile(w http.ResponseWriter, r *http.Request) {
	user := r.Context().Value("user").(*model.User)
	spaceId := r.PathValue("space_id")

	var req WriteFileRequest
	if err := rest.DecodeRequestBody(w, r, &req); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Invalid request body"})
		return
	}

	if req.Path == "" {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "path is required"})
		return
	}

	switch req.Mode {
	case "", "overwrite", "append", "prepend":
	default:
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid mode: " + req.Mode})
		return
	}

	db := database.GetInstance()
	var space *model.Space
	var err error
	if validate.UUID(spaceId) {
		space, err = db.GetSpace(spaceId)
	} else {
		space, err = db.GetSpaceByName(user.Id, spaceId)
	}
	if err != nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "Space not found"})
		return
	}
	spaceId = space.Id

	template, err := db.GetTemplate(space.TemplateId)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "Failed to get template"})
		return
	}

	if !template.WithRunCommand {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "File operations are not allowed in this space"})
		return
	}

	if space.UserId != user.Id && !space.IsSharedWith(user.Id) && !user.HasPermission(model.PermissionManageSpaces) {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "No permission to write files in this space"})
		return
	}

	if !space.IsDeployed {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "Space is not running"})
		return
	}

	session := agent_server.GetSession(spaceId)
	if session == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "Agent session not found for space"})
		return
	}

	copyCmd := &msg.CopyFileMessage{
		DestPath:      req.Path,
		Content:       []byte(req.Content),
		Direction:     "to_space",
		Workdir:       "",
		Mode:          req.Mode,
		MtimeNs:       req.MtimeNs,
		FilePerm:      req.FilePerm,
		SymlinkTarget: req.SymlinkTarget,
	}

	responseChannel, err := session.SendCopyFile(copyCmd)
	if err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: fmt.Sprintf("Failed to send write file command to agent: %v", err)})
		return
	}

	response := <-responseChannel
	if response == nil {
		rest.WriteResponse(http.StatusServiceUnavailable, w, r, ErrorResponse{Error: "No response from agent"})
		return
	}

	result := WriteFileResponse{
		Success: response.Success,
	}

	if !response.Success {
		result.Error = response.Error
	} else {
		result.BytesWritten = len(req.Content)
	}

	rest.WriteResponse(http.StatusOK, w, r, result)
}
