package util

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func DownloadUnpackTgz(downloadURL string, destDir string) error {

	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download: %v", resp.Status)
	}

	// Extract the tar.gz file
	gzipReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar file: %v", err)
		}

		target, err := sanitizeExtractPath(header.Name, destDir)
		if err != nil {
			return fmt.Errorf("invalid file path: %v", err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				return fmt.Errorf("failed to create directory: %v", err)
			}
		case tar.TypeReg:
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return fmt.Errorf("failed to create file: %v", err)
			}
			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return fmt.Errorf("failed to write file: %v", err)
			}
			file.Close()
		case tar.TypeSymlink:
			if isRel(header.Linkname, destDir) {
				// Ensure that the symlink itself is created within destDir after evaluating symlinks.
				ok, err := isPathWithinRoot(target, destDir)
				if err != nil {
					return fmt.Errorf("error resolving symlink destination: %v", err)
				}
				if !ok {
					return fmt.Errorf("symlink destination escapes extraction root: %s", target)
				}
				if err := os.Symlink(header.Linkname, target); err != nil {
					return fmt.Errorf("failed to create symlink: %v", err)
				}
			}
		case tar.TypeLink:
			linkTarget, err := sanitizeExtractPath(header.Linkname, destDir)
			if err != nil {
				return fmt.Errorf("invalid hard link target path: %v", err)
			}
			if err := os.Link(linkTarget, target); err != nil {
				return fmt.Errorf("failed to create hard link: %v", err)
			}
		default:
			return fmt.Errorf("unknown type: %v in %s", header.Typeflag, header.Name)
		}
	}

	return nil
}

// isPathWithinRoot checks that the fully resolved path (with all symlinks in parent directories evaluated)
// is still within the specified root directory.
func isPathWithinRoot(path, root string) (bool, error) {
	parent := filepath.Dir(path)
	// Evaluate symlinks in parent path
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		// If the directory does not exist yet, fallback to checking absolute path
		resolvedParent = parent
	}
	finalPath := filepath.Join(resolvedParent, filepath.Base(path))
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false, fmt.Errorf("cannot evaluate root: %v", err)
	}
	absFinal, err := filepath.Abs(finalPath)
	if err != nil {
		return false, fmt.Errorf("cannot evaluate path: %v", err)
	}
	rel, err := filepath.Rel(absRoot, absFinal)
	if err != nil {
		return false, nil
	}
	if strings.HasPrefix(filepath.Clean(rel), "..") {
		return false, nil
	}
	return true, nil
}

// isRel checks if the candidate path is a relative path within the target directory.
// It returns true if the candidate path is relative and resides within the target directory,
// otherwise it returns false.
//
// Parameters:
//   - candidate: The path to be checked.
//   - target: The base directory against which the candidate path is evaluated.
//
// The function first checks if the candidate path is absolute. If it is, the function returns false.
// If the candidate path is relative, the function resolves any symbolic links in the combined path
// of target and candidate. It then calculates the relative path from the target to the resolved path
// and checks if this relative path does not start with "..", indicating that the candidate path is
// within the target directory.
func isRel(candidate, target string) bool {
	if filepath.IsAbs(candidate) {
		return false
	}
	realpath, err := filepath.EvalSymlinks(filepath.Join(target, candidate))
	if err != nil {
		return false
	}
	relpath, err := filepath.Rel(target, realpath)
	return err == nil && !strings.HasPrefix(filepath.Clean(relpath), "..")
}

// sanitizeExtractPath ensures that the provided file path is clean, does not contain
// any directory traversal sequences, and is within the specified destination directory.
//
// Parameters:
// - filePath: The file path to sanitize.
// - destDir: The destination directory where the file should be extracted.
//
// Returns:
// - A sanitized absolute file path if the input path is valid and within the destination directory.
// - An error if the file path is invalid, contains directory traversal sequences, or is outside the destination directory.
func sanitizeExtractPath(filePath string, destDir string) (string, error) {
	// Ensure the file path is clean and does not contain any directory traversal sequences
	cleanPath := filepath.Clean(filePath)
	if strings.Contains(cleanPath, "..") {
		return "", fmt.Errorf("invalid file path: %s", filePath)
	}
	// Ensure the file path is within the destination directory
	absDestDir, err := filepath.Abs(destDir)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of destination directory: %v", err)
	}
	absFilePath, err := filepath.Abs(filepath.Join(destDir, cleanPath))
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path of file: %v", err)
	}
	if !strings.HasPrefix(absFilePath, absDestDir) {
		return "", fmt.Errorf("file path is outside the destination directory: %s", filePath)
	}
	return absFilePath, nil
}
