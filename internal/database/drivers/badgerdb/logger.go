package driver_badgerdb

import (
	"fmt"

	"github.com/paularlott/knot/internal/log"
)

// Replace the logger built into badger with our own
type badgerdbLog struct{}

func badgerdbLogger() *badgerdbLog {
	return &badgerdbLog{}
}

func (l *badgerdbLog) Errorf(f string, v ...interface{}) {
	log.Error(fmt.Sprintf("BadgerDB: "+f, v...))
}

func (l *badgerdbLog) Warningf(f string, v ...interface{}) {
	log.Warn(fmt.Sprintf("BadgerDB: "+f, v...))
}

func (l *badgerdbLog) Infof(f string, v ...interface{}) {
	log.Info(fmt.Sprintf("BadgerDB: "+f, v...))
}

func (l *badgerdbLog) Debugf(f string, v ...interface{}) {
	log.Debug(fmt.Sprintf("BadgerDB: "+f, v...))
}
