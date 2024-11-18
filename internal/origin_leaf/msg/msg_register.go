package msg

// message sent from an follower to the leader to register itself
type Register struct {
	Version  string
	Location string
}

// message sent from the leader to the follower in response to a register message
type RegisterResponse struct {
	Success bool
	Version string
}
