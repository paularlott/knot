package msg

const (
	MSG_TERMINAL_DATA = iota
	MSG_TERMINAL_RESIZE
)

// message for terminal
type Terminal struct {
	Shell string
}

type TerminalWindowSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
	X    uint16
	Y    uint16
}
