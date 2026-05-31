package container

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ManagedPathType = "path"

func ResolveManagedPath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path must not be empty")
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			return filepath.Clean(home), nil
		}
		return filepath.Clean(filepath.Join(home, path[2:])), nil
	}

	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}

	return filepath.Abs(path)
}

func CreateManagedPath(path string) (string, error) {
	resolved, err := ResolveManagedPath(path)
	if err != nil {
		return "", err
	}

	if err = os.MkdirAll(resolved, 0755); err != nil {
		return "", err
	}

	return resolved, nil
}

func DeleteManagedPath(path string) error {
	resolved, err := ResolveManagedPath(path)
	if err != nil {
		return err
	}
	if err = validateManagedPathDelete(resolved); err != nil {
		return err
	}

	return os.RemoveAll(resolved)
}

func validateManagedPathDelete(path string) error {
	path = filepath.Clean(path)
	if path == string(filepath.Separator) {
		return fmt.Errorf("refusing to delete root path %q", path)
	}

	if home, err := os.UserHomeDir(); err == nil && path == filepath.Clean(home) {
		return fmt.Errorf("refusing to delete home directory %q", path)
	}

	if cwd, err := os.Getwd(); err == nil && path == filepath.Clean(cwd) {
		return fmt.Errorf("refusing to delete working directory %q", path)
	}

	return nil
}
