package msg

// message sent to update the authorized keys within an agent
type UpdateAuthorizedKeys struct {
	SSHKeys         []string
	GitHubUsernames []string
}
