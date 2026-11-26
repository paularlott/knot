//go:build !darwin

package dirwatch

func newPlatform(opt Options) (Watcher, error) {
	return NewNotify(opt)
}
