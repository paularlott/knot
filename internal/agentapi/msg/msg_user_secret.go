package msg

import "net"

// UserSecretRequest asks the server for one of the space owner's personal
// secrets (Knot Pro). Personal secrets are only ever read over the agent
// link, so the server knows the space asking and can apply its template.
type UserSecretRequest struct {
	Path  string // vault/item, or a complete op:// reference
	Field string // defaults to password; may be section/field
}

// UserSecretResponse carries the value, or the reason it can't be read.
type UserSecretResponse struct {
	Value string
	Error string
}

// SendResolveUserSecret sends a personal secret request on a fresh stream
// and reads the reply.
func SendResolveUserSecret(conn net.Conn, path, field string) (*UserSecretResponse, error) {
	if err := WriteCommand(conn, CmdResolveUserSecret); err != nil {
		return nil, err
	}
	if err := WriteMessage(conn, &UserSecretRequest{Path: path, Field: field}); err != nil {
		return nil, err
	}

	var response UserSecretResponse
	if err := ReadMessage(conn, &response); err != nil {
		return nil, err
	}
	return &response, nil
}
