package msg

// SyncFlushResponse is the agent's answer to CmdSyncFlush: whether every
// local change its sync projects had not sent went.
type SyncFlushResponse struct {
	Error string `json:"error,omitempty" msgpack:"error,omitempty"`
}
