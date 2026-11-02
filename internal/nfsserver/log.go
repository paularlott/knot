package nfsserver

import (
	"fmt"
	"os"

	"github.com/paularlott/knot/internal/log"
	nfs "github.com/willscott/go-nfs"
)

// nfsLogger sends go-nfs's log to knot's at debug level. go-nfs logs every
// failed lookup as an error, which is routine for a filesystem, so its log
// is only shown when KNOT_NFS_DEBUG is set; the filesystem logs what matters
// itself.
type nfsLogger struct{ level nfs.LogLevel }

func init() {
	l := &nfsLogger{level: nfs.FatalLevel}
	if os.Getenv("KNOT_NFS_DEBUG") != "" {
		l.level = nfs.TraceLevel
	}
	nfs.SetLogger(l)
}

func (l *nfsLogger) log(level nfs.LogLevel, msg string) {
	if level <= l.level {
		log.WithGroup("nfs-protocol").Debug(msg)
	}
}

func (l *nfsLogger) SetLevel(level nfs.LogLevel) { l.level = level }
func (l *nfsLogger) GetLevel() nfs.LogLevel      { return l.level }
func (l *nfsLogger) ParseLevel(string) (nfs.LogLevel, error) {
	return l.level, nil
}

func (l *nfsLogger) Panic(args ...any) { l.log(nfs.PanicLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Fatal(args ...any) { l.log(nfs.FatalLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Error(args ...any) { l.log(nfs.ErrorLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Warn(args ...any)  { l.log(nfs.WarnLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Info(args ...any)  { l.log(nfs.InfoLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Debug(args ...any) { l.log(nfs.DebugLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Trace(args ...any) { l.log(nfs.TraceLevel, fmt.Sprint(args...)) }
func (l *nfsLogger) Print(args ...any) { l.log(nfs.InfoLevel, fmt.Sprint(args...)) }

func (l *nfsLogger) Panicf(f string, args ...any) { l.log(nfs.PanicLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Fatalf(f string, args ...any) { l.log(nfs.FatalLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Errorf(f string, args ...any) { l.log(nfs.ErrorLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Warnf(f string, args ...any)  { l.log(nfs.WarnLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Infof(f string, args ...any)  { l.log(nfs.InfoLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Debugf(f string, args ...any) { l.log(nfs.DebugLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Tracef(f string, args ...any) { l.log(nfs.TraceLevel, fmt.Sprintf(f, args...)) }
func (l *nfsLogger) Printf(f string, args ...any) { l.log(nfs.InfoLevel, fmt.Sprintf(f, args...)) }
