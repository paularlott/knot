//go:build !linux

package dirwatch

import (
	"errors"
	"syscall"
)

// LimitHint says why folders could not be watched and how to fix it.
func LimitHint(err error, folders int) string {
	switch {
	case errors.Is(err, syscall.EMFILE), errors.Is(err, syscall.ENFILE):
		return "the limit on open files is reached: raise it with ulimit -n"
	case err != nil:
		return err.Error()
	}
	return ""
}
