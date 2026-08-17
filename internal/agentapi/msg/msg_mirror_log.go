package msg

import "time"

// MirrorLogEntry is a single log record mirrored to a log sink space, tagged
// with the space it came from.
type MirrorLogEntry struct {
	SpaceId   string            `msgpack:"space_id"`
	SpaceName string            `msgpack:"space_name"`
	User      string            `msgpack:"user"` // owner of the source space
	Service   string            `msgpack:"service"`
	Level     LogLevel          `msgpack:"level"`
	Message   string            `msgpack:"message"`
	Date      time.Time         `msgpack:"date"`
	Fields    map[string]string `msgpack:"fields,omitempty"`
}

// MirrorLogMessage is a batch of log records sent from the server to a space
// registered as a log sink. The sink space's agent forwards the entries to
// the local log service on the port it advertised at registration, in the
// format it requested (vl | loki | gelf | json).
type MirrorLogMessage struct {
	Entries []*MirrorLogEntry `msgpack:"entries"`
}
