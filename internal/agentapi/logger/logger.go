package logger

import "github.com/rs/zerolog/log"

type MuxLogger struct {
}

func NewMuxLogger() *MuxLogger {
	return &MuxLogger{}
}

func (l *MuxLogger) Print(v ...interface{}) {
	log.Info().Msgf("%v", v...)
}

func (l *MuxLogger) Printf(format string, v ...interface{}) {
	log.Info().Msgf(format, v...)
}

func (l *MuxLogger) Println(v ...interface{}) {
	log.Info().Msgf("%v", v...)
}
