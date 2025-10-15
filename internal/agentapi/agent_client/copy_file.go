package agent_client

import (
	"net"
	"os"
	"path/filepath"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

func handleCopyFileExecution(stream net.Conn, copyCmd msg.CopyFileMessage) {
	log.Debug("agent: executing copy file", "direction", copyCmd.Direction, "source", copyCmd.SourcePath, "dest", copyCmd.DestPath)

	var response msg.CopyFileResponse

	switch copyCmd.Direction {
	case "to_space":
		response = handleCopyToSpace(copyCmd)
	case "from_space":
		response = handleCopyFromSpace(copyCmd)
	default:
		response = msg.CopyFileResponse{Success: false, Error: "Invalid direction"}
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		log.WithError(err).Error("agent: failed to send copy file response")
		return
	}

	log.Debug("agent: copy file execution completed", "error", response.Error, "success", response.Success)
}

func handleCopyToSpace(copyCmd msg.CopyFileMessage) msg.CopyFileResponse {
	destPath := copyCmd.DestPath

	// Handle relative paths
	if !filepath.IsAbs(destPath) && copyCmd.Workdir != "" {
		destPath = filepath.Join(copyCmd.Workdir, destPath)
	}

	// Create directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		log.Error("agent: failed to create directory", "error", err, "dir", destDir)
		return msg.CopyFileResponse{Success: false, Error: "Failed to create directory: " + err.Error()}
	}

	// Write file content
	if err := os.WriteFile(destPath, copyCmd.Content, 0644); err != nil {
		log.Error("agent: failed to write file", "error", err, "file", destPath)
		return msg.CopyFileResponse{Success: false, Error: "Failed to write file: " + err.Error()}
	}

	log.Debug("agent: file written successfully", "file", destPath, "bytes", len(copyCmd.Content))
	return msg.CopyFileResponse{Success: true}
}

func handleCopyFromSpace(copyCmd msg.CopyFileMessage) msg.CopyFileResponse {
	sourcePath := copyCmd.SourcePath

	// Handle relative paths
	if !filepath.IsAbs(sourcePath) && copyCmd.Workdir != "" {
		sourcePath = filepath.Join(copyCmd.Workdir, sourcePath)
	}

	// Read file content
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		log.Error("agent: failed to read file", "error", err, "file", sourcePath)
		if os.IsNotExist(err) {
			return msg.CopyFileResponse{Success: false, Error: "File not found: " + sourcePath}
		}
		return msg.CopyFileResponse{Success: false, Error: "Failed to read file: " + err.Error()}
	}

	// Get file permissions for logging
	if info, err := os.Stat(sourcePath); err == nil {
		log.Debug("agent: file read successfully", "file", sourcePath, "bytes", len(content), "mode", info.Mode().String())
	}

	return msg.CopyFileResponse{Success: true, Content: content}
}
