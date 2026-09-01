package msg

// message sent from the server to an agent whose Register carried a nonce:
// the agent must reply with a Register whose Proof is computed over both
// nonces, proving possession of the space's registration key without ever
// sending it.
type RegisterChallenge struct {
	ServerNonce string
}
