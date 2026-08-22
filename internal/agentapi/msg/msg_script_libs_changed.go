package msg

// ScriptLibsChanged notifies agents that a `lib`-type script changed on the
// server so they can drop their cached libs.zip package. UserId is set for a
// user library change (delivery targeted at that user's space agents) and
// empty for a global library change (broadcast to every agent).
type ScriptLibsChanged struct {
	UserId string `json:"user_id,omitempty"`
}
