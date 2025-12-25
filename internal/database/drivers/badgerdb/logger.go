package driver_badgerdb

import (
	"fmt"

	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/logger"
)

// Replace the logger built into badger with our own
type badgerdbLog struct {
	logger logger.Logger
}

func badgerdbLogger() *badgerdbLog {
	return &badgerdbLog{
		logger: log.WithGroup("db"),
	}
}

func (l *badgerdbLog) Errorf(f string, v ...interface{}) {
	l.logger.Error(fmt.Sprintf(f, v...))
}

func (l *badgerdbLog) Warningf(f string, v ...interface{}) {
	l.logger.Warn(fmt.Sprintf(f, v...))
}

func (l *badgerdbLog) Infof(f string, v ...interface{}) {
	l.logger.Info(fmt.Sprintf(f, v...))
}

func (l *badgerdbLog) Debugf(f string, v ...interface{}) {
}
