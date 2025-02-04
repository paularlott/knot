package util

import "os/exec"

func CheckShells(preferredShell string) string {
	shells := []string{preferredShell, "zsh", "bash", "sh"}
	for _, shell := range shells {
		if _, err := exec.LookPath(shell); err == nil {
			return shell
		}
	}
	return ""
}
