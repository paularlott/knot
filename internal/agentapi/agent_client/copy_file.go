package agent_client

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

func handleCopyFileExecution(stream net.Conn, copyCmd msg.CopyFileMessage) {
	logger := log.WithGroup("agent")
	logger.Debug("executing copy file", "direction", copyCmd.Direction, "source", copyCmd.SourcePath, "dest", copyCmd.DestPath)

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
		logger.WithError(err).Error("failed to send copy file response")
		return
	}

	logger.Debug("copy file execution completed", "error", response.Error, "success", response.Success)
}

func handleCopyToSpace(copyCmd msg.CopyFileMessage) msg.CopyFileResponse {
	logger := log.WithGroup("agent")
	destPath := copyCmd.DestPath

	// Handle relative paths
	if !filepath.IsAbs(destPath) && copyCmd.Workdir != "" {
		destPath = filepath.Join(copyCmd.Workdir, destPath)
	}

	// Symlink creation: when SymlinkTarget is set, create a symlink at
	// destPath pointing to the target. Remove any existing file/symlink
	// at the destination first (os.Symlink fails on EEXIST).
	if copyCmd.SymlinkTarget != "" {
		destDir := filepath.Dir(destPath)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return msg.CopyFileResponse{Success: false, Error: "Failed to create directory: " + err.Error()}
		}
		os.Remove(destPath) // ignore error — might not exist
		if err := os.Symlink(copyCmd.SymlinkTarget, destPath); err != nil {
			logger.Error("failed to create symlink", "error", err, "dest", destPath, "target", copyCmd.SymlinkTarget)
			return msg.CopyFileResponse{Success: false, Error: "Failed to create symlink: " + err.Error()}
		}
		logger.Debug("symlink created", "dest", destPath, "target", copyCmd.SymlinkTarget)
		return msg.CopyFileResponse{Success: true}
	}

	// Create directory if it doesn't exist
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		logger.Error("failed to create directory", "error", err, "dir", destDir)
		return msg.CopyFileResponse{Success: false, Error: "Failed to create directory: " + err.Error()}
	}

	// If the destination exists and isn't user-writable, open it up for the
	// overwrite/append/prepend below — otherwise os.WriteFile and OpenFile
	// fail with EPERM on read-only files (e.g. .git/objects/pack/*.pack,
	// which are 0444). When the caller set FilePerm in the message,
	// applySyncMetadata at the end restores the readonly mode.
	if info, err := os.Stat(destPath); err == nil && !info.IsDir() {
		if info.Mode().Perm()&0200 == 0 {
			if cerr := os.Chmod(destPath, info.Mode()|0200); cerr != nil {
				logger.Debug("could not make destination writable", "error", cerr, "file", destPath)
				// Fall through — the write itself may still succeed if the
				// parent dir allows unlink+recreate, or fail with a clear error.
			}
		}
	}

	mode := copyCmd.Mode
	if mode == "" {
		mode = "overwrite"
	}

	switch mode {
	case "overwrite":
		if err := os.WriteFile(destPath, copyCmd.Content, 0644); err != nil {
			logger.Error("failed to write file", "error", err, "file", destPath)
			return msg.CopyFileResponse{Success: false, Error: "Failed to write file: " + err.Error()}
		}

	case "append":
		f, err := os.OpenFile(destPath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			logger.Error("failed to open file for append", "error", err, "file", destPath)
			return msg.CopyFileResponse{Success: false, Error: "Failed to open file: " + err.Error()}
		}
		if _, err := f.Write(copyCmd.Content); err != nil {
			f.Close()
			logger.Error("failed to append to file", "error", err, "file", destPath)
			return msg.CopyFileResponse{Success: false, Error: "Failed to append: " + err.Error()}
		}
		f.Close()

	case "prepend":
		existing, err := os.ReadFile(destPath)
		if err != nil && !os.IsNotExist(err) {
			logger.Error("failed to read file for prepend", "error", err, "file", destPath)
			return msg.CopyFileResponse{Success: false, Error: "Failed to read file: " + err.Error()}
		}
		combined := append(copyCmd.Content, existing...)
		if err := os.WriteFile(destPath, combined, 0644); err != nil {
			logger.Error("failed to write file", "error", err, "file", destPath)
			return msg.CopyFileResponse{Success: false, Error: "Failed to write file: " + err.Error()}
		}

	default:
		return msg.CopyFileResponse{Success: false, Error: "Invalid mode: " + mode}
	}

	// Apply sync metadata (optional, used by knot sync tools to preserve source
	// semantics). Done after the write so it isn't clobbered by truncation.
	if err := applySyncMetadata(destPath, copyCmd); err != nil {
		logger.Warn("failed to apply sync metadata", "error", err, "file", destPath)
		// Non-fatal — the content is already written.
	}

	logger.Debug("file written successfully", "file", destPath, "bytes", len(copyCmd.Content), "mode", mode)
	return msg.CopyFileResponse{Success: true}
}

// applySyncMetadata applies the optional mtime and permission bits carried by
// CopyFileMessage. Both are no-ops when zero. Applied AFTER the content write
// so they reflect the synced state, not whatever the FS picked for a fresh file.
func applySyncMetadata(destPath string, copyCmd msg.CopyFileMessage) error {
	if copyCmd.FilePerm != 0 {
		if err := os.Chmod(destPath, os.FileMode(copyCmd.FilePerm)); err != nil {
			return err
		}
	}
	if copyCmd.MtimeNs != 0 {
		t := time.Unix(0, copyCmd.MtimeNs)
		if err := os.Chtimes(destPath, t, t); err != nil {
			return err
		}
	}
	return nil
}

func handleCopyFromSpace(copyCmd msg.CopyFileMessage) msg.CopyFileResponse {
	logger := log.WithGroup("agent")
	sourcePath := copyCmd.SourcePath

	// Handle relative paths
	if !filepath.IsAbs(sourcePath) && copyCmd.Workdir != "" {
		sourcePath = filepath.Join(copyCmd.Workdir, sourcePath)
	}

	// Read file content
	content, err := os.ReadFile(sourcePath)
	if err != nil {
		logger.Error("failed to read file", "error", err, "file", sourcePath)
		if os.IsNotExist(err) {
			return msg.CopyFileResponse{Success: false, Error: "File not found: " + sourcePath}
		}
		return msg.CopyFileResponse{Success: false, Error: "Failed to read file: " + err.Error()}
	}

	// Get file permissions for logging
	if info, err := os.Stat(sourcePath); err == nil {
		logger.Debug("file read successfully", "file", sourcePath, "bytes", len(content), "mode", info.Mode().String())
	}

	return msg.CopyFileResponse{Success: true, Content: content}
}
