package logger

import (
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/log"
)

type MuxLogger struct {
}

func NewMuxLogger() *MuxLogger {
	return &MuxLogger{}
}

func (l *MuxLogger) Print(v ...interface{}) {
	log.Info(fmt.Sprint(v...))
}

func (l *MuxLogger) Printf(format string, v ...interface{}) {
	// Skip logging expected websocket closure messages
	if strings.Contains(format, "Failed to read header") &&
		len(v) > 0 &&
		strings.Contains(fmt.Sprintf("%v", v[0]), "websocket: close 1006") {
		return
	}

	log.Info(fmt.Sprintf(format, v...))
}

func (l *MuxLogger) Println(v ...interface{}) {
	log.Info(fmt.Sprint(v...))
}
