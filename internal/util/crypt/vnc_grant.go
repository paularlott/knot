package crypt

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// VNCGrant lets a browser into one web VNC host on behalf of a knot web
// session. The session cookie stays on knot's own host; the VNC host gets
// its own cookie instead, holding a VNCGrant.
//
// A grant is sealed with AES-GCM, so the session ID inside it never shows,
// and bound to the VNC host, so it opens nowhere else.
type VNCGrant struct {
	Kind      string `json:"k" msgpack:"k"` // VNCGrantHandoff or VNCGrantCookie
	SessionId string `json:"s" msgpack:"s"`
	Expires   int64  `json:"e,omitempty" msgpack:"e,omitempty"` // unix seconds; 0 = the session's own life
	Nonce     string `json:"n" msgpack:"n"`
}

const (
	// VNCGrantHandoff is the single-use code knot's host hands the VNC
	// host in a redirect.
	VNCGrantHandoff = "handoff"
	// VNCGrantCookie is what the VNC host keeps in its cookie.
	VNCGrantCookie = "cookie"
)

// SealVNCGrant encrypts a grant for the given VNC host.
func SealVNCGrant(encryptionKey, host string, g VNCGrant) (string, error) {
	if g.Nonce == "" {
		nonce, err := randomB64URL(16)
		if err != nil {
			return "", err
		}
		g.Nonce = nonce
	}
	plain, err := json.Marshal(g)
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(encryptionKey)
	if err != nil {
		return "", err
	}
	iv := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(iv, iv, plain, vncGrantAAD(host))), nil
}

// OpenVNCGrant decrypts a grant, failing for one altered or sealed for
// another host. The caller checks its kind, expiry and session.
func OpenVNCGrant(encryptionKey, host, sealed string) (VNCGrant, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil {
		return VNCGrant{}, false
	}
	gcm, err := newGCM(encryptionKey)
	if err != nil || len(raw) < gcm.NonceSize() {
		return VNCGrant{}, false
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], vncGrantAAD(host))
	if err != nil {
		return VNCGrant{}, false
	}
	var g VNCGrant
	if err := json.Unmarshal(plain, &g); err != nil || g.SessionId == "" || g.Nonce == "" {
		return VNCGrant{}, false
	}
	return g, true
}

// vncGrantAAD binds a grant to its host, and keeps it from opening as any
// other value sealed with the same key.
func vncGrantAAD(host string) []byte {
	return []byte("knot vnc grant\x00" + strings.ToLower(host))
}
