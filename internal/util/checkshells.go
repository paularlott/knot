package util

import "os/exec"

func CheckShells(preferredShell string) string {
	shells := []string{preferredShell, "zsh", "bash", "sh"}
	for _, shell := range shells {
		if path, err := exec.LookPath(shell); err == nil {
			return path
		}
	}
	return ""
}
