package agent_client

import (
	"net"
	"os"
	"path/filepath"

	"github.com/paularlott/knot/internal/agentapi/msg"

	"github.com/paularlott/knot/internal/log"
)

func handleDeleteFileExecution(stream net.Conn, deleteCmd msg.DeleteFileMessage) {
	logger := log.WithGroup("agent")
	logger.Debug("executing delete file", "path", deleteCmd.Path, "recursive", deleteCmd.Recursive)

	var response msg.DeleteFileResponse

	targetPath := deleteCmd.Path
	if !filepath.IsAbs(targetPath) && deleteCmd.Workdir != "" {
		targetPath = filepath.Join(deleteCmd.Workdir, targetPath)
	}

	info, err := os.Stat(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Idempotent: a missing path is already in the desired state.
			response.Success = true
		} else {
			logger.Error("failed to stat path for delete", "error", err, "path", targetPath)
			response.Error = "Failed to stat path: " + err.Error()
		}
	} else if info.IsDir() && deleteCmd.Recursive {
		count, err := countTree(targetPath)
		if err != nil {
			logger.Error("failed to count tree for delete", "error", err, "path", targetPath)
			response.Error = "Failed to count tree: " + err.Error()
		} else if err := os.RemoveAll(targetPath); err != nil {
			logger.Error("failed to remove directory", "error", err, "path", targetPath)
			response.Error = "Failed to remove directory: " + err.Error()
		} else {
			response.Success = true
			response.Removed = count
		}
	} else if info.IsDir() {
		// Non-recursive directory delete: only succeeds if the directory is empty.
		if err := os.Remove(targetPath); err != nil {
			logger.Error("failed to remove directory (non-recursive)", "error", err, "path", targetPath)
			response.Error = "Failed to remove directory (use recursive for non-empty dirs): " + err.Error()
		} else {
			response.Success = true
			response.Removed = 1
		}
	} else {
		if err := os.Remove(targetPath); err != nil {
			logger.Error("failed to remove file", "error", err, "path", targetPath)
			response.Error = "Failed to remove file: " + err.Error()
		} else {
			response.Success = true
			response.Removed = 1
		}
	}

	if err := msg.WriteMessage(stream, &response); err != nil {
		logger.WithError(err).Error("failed to send delete file response")
		return
	}

	logger.Debug("delete file execution completed", "error", response.Error, "success", response.Success, "removed", response.Removed)
}

// countTree returns the number of filesystem entries (files + directories)
// under root, including root itself. Used to populate DeleteFileResponse.Removed
// for recursive deletes so the caller gets meaningful feedback.
func countTree(root string) (int, error) {
	count := 0
	err := filepath.WalkDir(root, func(_ string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		return nil
	})
	return count, err
}
