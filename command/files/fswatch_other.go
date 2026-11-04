//go:build !darwin

package command_files

// newDirWatcher watches the directory with fsnotify.
func newDirWatcher(s *syncer) (dirWatcher, error) {
	return newNotifyWatcher(s)
}
