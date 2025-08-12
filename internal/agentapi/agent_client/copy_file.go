package agent_client

import (
	"net"
	"os"
	"path/filepath"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/rs/zerolog/log"
)

func handleCopyFileExecution(stream net.Conn, copyCmd msg.CopyFileMessage) {
	log.Debug().Str("direction", copyCmd.Direction).Str("source", copyCmd.SourcePath).Str("dest", copyCmd.DestPath).Msg("agent: executing copy file")

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
		log.Error().Err(err).Msg("agent: failed to send copy file response")
		return
	}

	log.Debug().Bool("success", response.Success).Str("error", response.Error).Msg("agent: copy file execution completed")
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
		log.Error().Err(err).Str("dir", destDir).Msg("agent: failed to create directory")
		return msg.CopyFileResponse{Success: false, Error: "Failed to create directory: " + err.Error()}
	}

	// Write file content
	if err := os.WriteFile(destPath, copyCmd.Content, 0644); err != nil {
		log.Error().Err(err).Str("file", destPath).Msg("agent: failed to write file")
		return msg.CopyFileResponse{Success: false, Error: "Failed to write file: " + err.Error()}
	}

	log.Debug().Str("file", destPath).Int("bytes", len(copyCmd.Content)).Msg("agent: file written successfully")
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
		log.Error().Err(err).Str("file", sourcePath).Msg("agent: failed to read file")
		if os.IsNotExist(err) {
			return msg.CopyFileResponse{Success: false, Error: "File not found: " + sourcePath}
		}
		return msg.CopyFileResponse{Success: false, Error: "Failed to read file: " + err.Error()}
	}

	// Get file permissions for logging
	if info, err := os.Stat(sourcePath); err == nil {
		log.Debug().Str("file", sourcePath).Int("bytes", len(content)).Str("mode", info.Mode().String()).Msg("agent: file read successfully")
	}

	return msg.CopyFileResponse{Success: true, Content: content}
}
