package msg

// message sent to update the authorized keys within an agent
type UpdateAuthorizedKeys struct {
	SSHKey         string
	GitHubUsername string
}
