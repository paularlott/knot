package driver_badgerdb

import (
	"github.com/rs/zerolog/log"
)

// Replace the logger built into badger with our own
type badgerdbLog struct {}

func badgerdbLogger() *badgerdbLog {
	return &badgerdbLog{}
}

func (l *badgerdbLog) Errorf(f string, v ...interface{}) {
  log.Error().Msgf("BadgerDB: " + f, v ...)
}

func (l *badgerdbLog) Warningf(f string, v ...interface{}) {
  log.Warn().Msgf("BadgerDB: " + f, v ...)
}

func (l *badgerdbLog) Infof(f string, v ...interface{}) {
  log.Info().Msgf("BadgerDB: " + f, v ...)
}

func (l *badgerdbLog) Debugf(f string, v ...interface{}) {
  log.Debug().Msgf("BadgerDB: " + f, v ...)
}
