//go:build !windows

package util

// HideConsoleIfOwned is a no-op outside Windows.
func HideConsoleIfOwned() {}

// RelaunchHidden is a no-op outside Windows.
func RelaunchHidden() bool { return false }
