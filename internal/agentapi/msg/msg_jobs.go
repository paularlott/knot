package msg

// JobsRunMessage asks the agent to start a job immediately by name. Sent from
// the server to the agent.
type JobsRunMessage struct {
	Name string `json:"name" msgpack:"name"`
}

// JobsSetEnabledMessage starts or stops the job runner. Sent from the
// server to the agent.
type JobsSetEnabledMessage struct {
	Enabled bool `json:"enabled" msgpack:"enabled"`
}

// JobsResponse is the outcome of a run or set-enabled request.
type JobsResponse struct {
	Success bool   `json:"success" msgpack:"success"`
	Error   string `json:"error,omitempty" msgpack:"error,omitempty"`
}
