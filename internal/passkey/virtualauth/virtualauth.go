// Package virtualauth is a software WebAuthn authenticator for tests: it produces "none" attestations and ES256
// assertions exactly as the specification lays them out, so ceremonies can be driven end to end without a browser.
package virtualauth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

var b64 = base64.RawURLEncoding

// Credential is one resident credential held by the authenticator.
type Credential struct {
	ID         []byte
	Key        *ecdsa.PrivateKey
	RPID       string
	UserHandle []byte
	Counter    uint32
}

// Authenticator holds credentials and the knobs tests turn to exercise failure paths.
type Authenticator struct {
	// Origin is what the "browser" reports in clientDataJSON (default: the origin passed to New).
	Origin string
	// UserVerified sets the UV flag (default true); BackupEligible/BackupState set BE/BS.
	UserVerified   bool
	BackupEligible bool
	BackupState    bool
	// AAGUID is reported at registration.
	AAGUID [16]byte
	// Counter increment per assertion; 0 keeps the counter at zero like most synced passkeys.
	CounterStep uint32
	Creds       []*Credential
}

// New returns an authenticator for a browser at origin.
func New(origin string) *Authenticator {
	return &Authenticator{Origin: origin, UserVerified: true, CounterStep: 1}
}

type creationOptions struct {
	PublicKey struct {
		Challenge string `json:"challenge" msgpack:"challenge"`
		RP        struct {
			ID string `json:"id" msgpack:"id"`
		} `json:"rp" msgpack:"rp"`
		User struct {
			ID string `json:"id" msgpack:"id"`
		} `json:"user" msgpack:"user"`
		ExcludeCredentials []struct {
			ID string `json:"id" msgpack:"id"`
		} `json:"excludeCredentials" msgpack:"excludeCredentials"`
	} `json:"publicKey" msgpack:"publicKey"`
}

type requestOptions struct {
	PublicKey struct {
		Challenge        string `json:"challenge" msgpack:"challenge"`
		RPID             string `json:"rpId" msgpack:"rpId"`
		AllowCredentials []struct {
			ID string `json:"id" msgpack:"id"`
		} `json:"allowCredentials" msgpack:"allowCredentials"`
	} `json:"publicKey" msgpack:"publicKey"`
}

func (a *Authenticator) flags(at bool) byte {
	f := byte(0x01)
	if a.UserVerified {
		f |= 0x04
	}
	if a.BackupEligible {
		f |= 0x08
	}
	if a.BackupState {
		f |= 0x10
	}
	if at {
		f |= 0x40
	}
	return f
}

func clientData(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin, "crossOrigin": false})
	return b
}

// Create answers navigator.credentials.create(options): options is the server's JSON ({"publicKey": ...}); the
// result is PublicKeyCredential.toJSON().
func (a *Authenticator) Create(options []byte) ([]byte, *Credential, error) {
	var o creationOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, nil, err
	}
	pk := o.PublicKey
	uh, err := b64.DecodeString(pk.User.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("user id: %w", err)
	}
	for _, ex := range pk.ExcludeCredentials {
		id, _ := b64.DecodeString(ex.ID)
		for _, c := range a.Creds {
			if bytes.Equal(c.ID, id) {
				return nil, nil, errors.New("virtualauth: credential excluded")
			}
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	c := &Credential{ID: make([]byte, 32), Key: key, RPID: pk.RP.ID, UserHandle: uh}
	_, _ = rand.Read(c.ID)
	a.Creds = append(a.Creds, c)

	rpHash := sha256.Sum256([]byte(pk.RP.ID))
	var ad bytes.Buffer
	ad.Write(rpHash[:])
	ad.WriteByte(a.flags(true))
	_ = binary.Write(&ad, binary.BigEndian, uint32(0))
	ad.Write(a.AAGUID[:])
	_ = binary.Write(&ad, binary.BigEndian, uint16(len(c.ID)))
	ad.Write(c.ID)
	ad.Write(coseKey(key))

	var att bytes.Buffer
	att.WriteByte(0xa3) // map(3)
	cborText(&att, "fmt")
	cborText(&att, "none")
	cborText(&att, "attStmt")
	att.WriteByte(0xa0) // map(0)
	cborText(&att, "authData")
	cborBytes(&att, ad.Bytes())

	cd := clientData("webauthn.create", pk.Challenge, a.Origin)
	out := map[string]any{
		"id": b64.EncodeToString(c.ID), "rawId": b64.EncodeToString(c.ID), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(cd), "attestationObject": b64.EncodeToString(att.Bytes()),
			"transports": []string{"internal"},
		},
		"clientExtensionResults": map[string]any{},
	}
	b, err := json.Marshal(out)
	return b, c, err
}

// Get answers navigator.credentials.get(options). The credential is chosen from the allow list, or, when empty,
// any resident credential for the RP. The result is PublicKeyCredential.toJSON().
func (a *Authenticator) Get(options []byte) ([]byte, *Credential, error) {
	var o requestOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, nil, err
	}
	pk := o.PublicKey
	var c *Credential
	for _, cand := range a.Creds {
		if cand.RPID != pk.RPID {
			continue
		}
		if len(pk.AllowCredentials) > 0 {
			ok := slices.ContainsFunc(pk.AllowCredentials, func(ac struct {
				ID string `json:"id" msgpack:"id"`
			}) bool {
				return ac.ID == b64.EncodeToString(cand.ID)
			})
			if !ok {
				continue
			}
		}
		c = cand
	}
	if c == nil {
		return nil, nil, errors.New("virtualauth: no matching credential")
	}
	return a.assert(c, pk.Challenge, a.Origin, c.Counter+a.CounterStep)
}

// GetWith signs an assertion with an explicit counter and origin (to forge regressions and wrong origins).
func (a *Authenticator) GetWith(options []byte, c *Credential, counter uint32, origin string) ([]byte, error) {
	var o requestOptions
	if err := json.Unmarshal(options, &o); err != nil {
		return nil, err
	}
	b, _, err := a.assert(c, o.PublicKey.Challenge, origin, counter)
	return b, err
}

func (a *Authenticator) assert(c *Credential, challenge, origin string, counter uint32) ([]byte, *Credential, error) {
	c.Counter = counter
	rpHash := sha256.Sum256([]byte(c.RPID))
	var ad bytes.Buffer
	ad.Write(rpHash[:])
	ad.WriteByte(a.flags(false))
	_ = binary.Write(&ad, binary.BigEndian, counter)
	cd := clientData("webauthn.get", challenge, origin)
	cdh := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad.Bytes()...), cdh[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, c.Key, digest[:])
	if err != nil {
		return nil, nil, err
	}
	out := map[string]any{
		"id": b64.EncodeToString(c.ID), "rawId": b64.EncodeToString(c.ID), "type": "public-key",
		"authenticatorAttachment": "platform",
		"response": map[string]any{
			"clientDataJSON": b64.EncodeToString(cd), "authenticatorData": b64.EncodeToString(ad.Bytes()),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString(c.UserHandle),
		},
		"clientExtensionResults": map[string]any{},
	}
	b, err := json.Marshal(out)
	return b, c, err
}

// ---------------------------------------------------------------------------------------------------------------
// Minimal CBOR (RFC 8949) for the few shapes WebAuthn needs.

func cborHead(b *bytes.Buffer, major byte, n uint64) {
	switch {
	case n < 24:
		b.WriteByte(major<<5 | byte(n))
	case n < 256:
		b.WriteByte(major<<5 | 24)
		b.WriteByte(byte(n))
	case n < 65536:
		b.WriteByte(major<<5 | 25)
		_ = binary.Write(b, binary.BigEndian, uint16(n))
	default:
		b.WriteByte(major<<5 | 26)
		_ = binary.Write(b, binary.BigEndian, uint32(n))
	}
}

func cborText(b *bytes.Buffer, s string)  { cborHead(b, 3, uint64(len(s))); b.WriteString(s) }
func cborBytes(b *bytes.Buffer, p []byte) { cborHead(b, 2, uint64(len(p))); b.Write(p) }
func cborInt(b *bytes.Buffer, n int64) {
	if n >= 0 {
		cborHead(b, 0, uint64(n))
	} else {
		cborHead(b, 1, uint64(-1-n))
	}
}

// coseKey encodes an EC2 P-256 ES256 public key as a COSE_Key.
func coseKey(k *ecdsa.PrivateKey) []byte {
	var b bytes.Buffer
	b.WriteByte(0xa5) // map(5)
	cborInt(&b, 1)
	cborInt(&b, 2) // kty: EC2
	cborInt(&b, 3)
	cborInt(&b, -7) // alg: ES256
	cborInt(&b, -1)
	cborInt(&b, 1) // crv: P-256
	cborInt(&b, -2)
	cborBytes(&b, k.PublicKey.X.FillBytes(make([]byte, 32)))
	cborInt(&b, -3)
	cborBytes(&b, k.PublicKey.Y.FillBytes(make([]byte, 32)))
	return b.Bytes()
}
