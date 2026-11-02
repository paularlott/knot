// Package passkey runs the WebAuthn registration and sign-in ceremonies on
// top of go-webauthn.
//
// The relying party ID is the host of the server's public URL and the only
// accepted origin is that URL, so a server reached by IP address (which
// WebAuthn forbids as an RP ID) has no passkeys; Enabled reports that.
//
// A begun ceremony is kept in a short-lived cookie signed with the server's
// encryption key, so the finish can land on any server in the zone. Each
// challenge is accepted once per server: a finish remembers the challenge
// until it expires and refuses it after that.
//
// The WebAuthn user handle is the user id's bytes, so a discoverable sign-in
// reads the user from the handle and then finds the credential among that
// user's passkeys.
package passkey

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/middleware"
)

// ChallengeTTL is how long a begun ceremony stays valid.
const ChallengeTTL = 5 * time.Minute

// MaxPasskeys caps the passkeys one user may register.
const MaxPasskeys = 20

const (
	cookieLogin    = "__KNOT_PASSKEY_LOGIN"
	cookieRegister = "__KNOT_PASSKEY_REGISTER"
)

var (
	// ErrDisabled means the server's URL cannot be a relying party (no host, or an IP address).
	ErrDisabled = errors.New("passkeys are not available on this server")
	// ErrChallenge means the ceremony is unknown, expired, already used or for someone else.
	ErrChallenge = errors.New("the passkey request expired, try again")
	// ErrVerify means the authenticator's response did not verify.
	ErrVerify = errors.New("the passkey could not be verified")
	// ErrUnknownCredential means no registered passkey matches the response.
	ErrUnknownCredential = errors.New("this passkey is not registered")
	// ErrCloneDetected means the sign counter did not increase: the credential may have been copied.
	ErrCloneDetected = errors.New("this passkey's counter went backwards, it may have been copied")
	// ErrInactive means the passkey's user is disabled or deleted.
	ErrInactive = errors.New("the account is not active")
	// ErrTooMany means the user already has MaxPasskeys passkeys.
	ErrTooMany = fmt.Errorf("an account can have at most %d passkeys", MaxPasskeys)
)

var (
	waMu  sync.Mutex
	waURL string
	waRP  *webauthn.WebAuthn
)

// relyingParty returns the WebAuthn instance for the server's current URL.
func relyingParty() (*webauthn.WebAuthn, error) {
	cfg := config.GetServerConfig()

	// A leaf takes its users from the origin, which would overwrite any
	// passkey registered on the leaf.
	if cfg.LeafNode {
		return nil, ErrDisabled
	}

	waMu.Lock()
	defer waMu.Unlock()
	if waRP != nil && waURL == cfg.URL {
		return waRP, nil
	}

	u, err := url.Parse(cfg.URL)
	if err != nil || u.Hostname() == "" || net.ParseIP(u.Hostname()) != nil {
		return nil, ErrDisabled
	}
	wa, err := webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "knot",
		RPOrigins:     []string{u.Scheme + "://" + u.Host},
		// Passkeys here are sign-in credentials, not device attestations.
		AttestationPreference: protocol.PreferNoAttestation,
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementRequired,
			// Required, as at sign-in: a key that can't verify its user (a
			// security key without a PIN) could register but never sign in.
			UserVerification: protocol.VerificationRequired,
		},
		// Enforced so every ceremony carries an expiry, which take checks.
		Timeouts: webauthn.TimeoutsConfig{
			Login:        webauthn.TimeoutConfig{Enforce: true, Timeout: ChallengeTTL, TimeoutUVD: ChallengeTTL},
			Registration: webauthn.TimeoutConfig{Enforce: true, Timeout: ChallengeTTL, TimeoutUVD: ChallengeTTL},
		},
	})
	if err != nil {
		return nil, ErrDisabled
	}
	waURL, waRP = cfg.URL, wa
	return wa, nil
}

// Enabled reports whether passkeys can be used with the server's URL.
func Enabled() bool {
	_, err := relyingParty()
	return err == nil
}

// Origin returns the one origin passkeys work at, the server's URL without
// its path, or "" when passkeys are not available.
func Origin() string {
	wa, err := relyingParty()
	if err != nil {
		return ""
	}
	return wa.Config.RPOrigins[0]
}

// ---------------------------------------------------------------------------
// User adapter

type waUser struct {
	u *model.User
}

func (w *waUser) WebAuthnID() []byte          { return []byte(w.u.Id) }
func (w *waUser) WebAuthnName() string        { return w.u.Username }
func (w *waUser) WebAuthnDisplayName() string { return w.u.Username }
func (w *waUser) WebAuthnCredentials() []webauthn.Credential {
	creds := make([]webauthn.Credential, 0, len(w.u.Passkeys))
	for i := range w.u.Passkeys {
		creds = append(creds, toCredential(&w.u.Passkeys[i]))
	}
	return creds
}

// authenticator data flag bits (WebAuthn §6.1).
const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
)

func toCredential(p *model.Passkey) webauthn.Credential {
	// Every stored passkey was registered with user verification.
	flags := byte(flagUP | flagUV)
	if p.BackupEligible {
		flags |= flagBE
	}
	if p.BackupState {
		flags |= flagBS
	}
	tr := make([]protocol.AuthenticatorTransport, len(p.Transports))
	for i, t := range p.Transports {
		tr[i] = protocol.AuthenticatorTransport(t)
	}
	return webauthn.Credential{
		ID: p.CredentialID, PublicKey: p.PublicKey, AttestationType: "none", AttestationFormat: p.AttestationFmt,
		Transport: tr, Flags: webauthn.NewCredentialFlags(protocol.AuthenticatorFlags(flags)),
		Authenticator: webauthn.Authenticator{AAGUID: p.AAGUID, SignCount: p.SignCount},
	}
}

// ---------------------------------------------------------------------------
// Ceremony state

type ceremony struct {
	Session webauthn.SessionData `json:"s" msgpack:"s"`
	// UserId binds a registration to the user who began it.
	UserId string `json:"u,omitempty" msgpack:"u,omitempty"`
}

func sign(data []byte) []byte {
	h := hmac.New(sha256.New, []byte("knot-passkey:"+config.GetServerConfig().EncryptionKey))
	h.Write(data)
	return h.Sum(nil)
}

func save(w http.ResponseWriter, r *http.Request, name string, c *ceremony) error {
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	value := base64.RawURLEncoding.EncodeToString(data) + "." + base64.RawURLEncoding.EncodeToString(sign(data))
	middleware.SetCookie(w, r, name, value, int(ChallengeTTL/time.Second))
	return nil
}

// usedChallenges remembers the challenges this server has finished, until
// they expire, so a response can't be replayed against the same ceremony.
var (
	usedMu         sync.Mutex
	usedChallenges = map[string]time.Time{}
)

func markUsed(challenge string, expires time.Time) bool {
	usedMu.Lock()
	defer usedMu.Unlock()

	now := time.Now()
	for k, exp := range usedChallenges {
		if now.After(exp) {
			delete(usedChallenges, k)
		}
	}
	if _, used := usedChallenges[challenge]; used {
		return false
	}
	usedChallenges[challenge] = expires
	return true
}

// take reads and clears the ceremony cookie; the ceremony can't be finished
// again whatever happens next.
func take(w http.ResponseWriter, r *http.Request, name string) (*ceremony, error) {
	cookie, err := middleware.GetCookie(r, name)
	middleware.ExpireCookie(w, r, name)
	if err != nil {
		return nil, ErrChallenge
	}
	enc, mac, ok := strings.Cut(cookie.Value, ".")
	if !ok {
		return nil, ErrChallenge
	}
	data, err1 := base64.RawURLEncoding.DecodeString(enc)
	sig, err2 := base64.RawURLEncoding.DecodeString(mac)
	if err1 != nil || err2 != nil || !hmac.Equal(sig, sign(data)) {
		return nil, ErrChallenge
	}
	var c ceremony
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, ErrChallenge
	}
	if c.Session.Expires.IsZero() || time.Now().After(c.Session.Expires) || !markUsed(c.Session.Challenge, c.Session.Expires) {
		return nil, ErrChallenge
	}
	return &c, nil
}

// ---------------------------------------------------------------------------
// Registration

// BeginRegistration starts enrolling a passkey for user. The user's existing
// credentials are excluded so one authenticator isn't registered twice.
func BeginRegistration(w http.ResponseWriter, r *http.Request, user *model.User) (*protocol.CredentialCreation, error) {
	wa, err := relyingParty()
	if err != nil {
		return nil, err
	}
	if len(user.Passkeys) >= MaxPasskeys {
		return nil, ErrTooMany
	}
	wu := &waUser{u: user}
	creation, sd, err := wa.BeginRegistration(wu,
		webauthn.WithExclusions(webauthn.Credentials(wu.WebAuthnCredentials()).CredentialDescriptors()),
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
	)
	if err != nil {
		return nil, err
	}
	if err := save(w, r, cookieRegister, &ceremony{Session: *sd, UserId: user.Id}); err != nil {
		return nil, err
	}
	return creation, nil
}

// FinishRegistration verifies the authenticator's attestation response (the
// JSON of PublicKeyCredential.toJSON()) and adds the passkey to user under
// name. The caller saves the user.
func FinishRegistration(w http.ResponseWriter, r *http.Request, user *model.User, credential []byte, name string) (*model.Passkey, error) {
	wa, err := relyingParty()
	if err != nil {
		return nil, err
	}
	c, err := take(w, r, cookieRegister)
	if err != nil {
		return nil, err
	}
	if c.UserId != user.Id {
		return nil, ErrChallenge
	}
	if len(user.Passkeys) >= MaxPasskeys {
		return nil, ErrTooMany
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(credential)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVerify, err)
	}
	cred, err := wa.CreateCredential(&waUser{u: user}, c.Session, parsed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVerify, err)
	}
	if !cred.Flags.UserVerified {
		return nil, fmt.Errorf("%w: the authenticator did not verify you, set up a PIN or biometrics on it", ErrVerify)
	}
	for _, p := range user.Passkeys {
		if bytes.Equal(p.CredentialID, cred.ID) {
			return nil, fmt.Errorf("%w: this passkey is already registered", ErrVerify)
		}
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	if len([]rune(name)) > 64 {
		name = string([]rune(name)[:64])
	}
	tr := make([]string, len(cred.Transport))
	for i, t := range cred.Transport {
		tr[i] = string(t)
	}
	id, err := uuid.NewV7()
	if err != nil {
		return nil, err
	}
	pk := model.Passkey{
		Id: id.String(), Name: name, CredentialID: cred.ID, PublicKey: cred.PublicKey,
		AAGUID: cred.Authenticator.AAGUID, AttestationFmt: cred.AttestationFormat, SignCount: cred.Authenticator.SignCount,
		Transports: tr, BackupEligible: cred.Flags.BackupEligible, BackupState: cred.Flags.BackupState,
		CreatedAt: time.Now().UTC(),
	}
	user.Passkeys = append(user.Passkeys, pk)
	return &pk, nil
}

// ---------------------------------------------------------------------------
// Sign-in

// BeginLogin starts a discoverable sign-in: the authenticator offers its
// passkeys for this server and the user picks one. User verification is
// required, so a passkey is a complete sign-in on its own.
func BeginLogin(w http.ResponseWriter, r *http.Request) (*protocol.CredentialAssertion, error) {
	wa, err := relyingParty()
	if err != nil {
		return nil, err
	}
	assertion, sd, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, err
	}
	if err := save(w, r, cookieLogin, &ceremony{Session: *sd}); err != nil {
		return nil, err
	}
	return assertion, nil
}

// LoginResult is a verified passkey sign-in. The passkey's counters are
// updated on User, which the caller saves.
type LoginResult struct {
	User    *model.User
	Passkey *model.Passkey
}

// FinishLogin verifies an assertion (PublicKeyCredential.toJSON()) against
// the begun ceremony and checks the sign counter.
func FinishLogin(w http.ResponseWriter, r *http.Request, credential []byte) (*LoginResult, error) {
	wa, err := relyingParty()
	if err != nil {
		return nil, err
	}
	c, err := take(w, r, cookieLogin)
	if err != nil {
		return nil, err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(credential)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrVerify, err)
	}

	var user *model.User
	_, cred, err := wa.ValidatePasskeyLogin(func(rawID, userHandle []byte) (webauthn.User, error) {
		u, uerr := database.GetInstance().GetUser(string(userHandle))
		if uerr != nil || u == nil || u.IsDeleted {
			return nil, ErrUnknownCredential
		}
		if !hasCredential(u, rawID) {
			return nil, ErrUnknownCredential
		}
		if !u.Active {
			return nil, ErrInactive
		}
		user = u
		return &waUser{u: u}, nil
	}, c.Session, parsed)
	if err != nil {
		switch {
		case errors.Is(err, ErrInactive):
			return nil, ErrInactive
		case errors.Is(err, ErrUnknownCredential):
			return nil, ErrUnknownCredential
		}
		return nil, fmt.Errorf("%w: %v", ErrVerify, err)
	}

	var pk *model.Passkey
	for i := range user.Passkeys {
		if bytes.Equal(user.Passkeys[i].CredentialID, cred.ID) {
			pk = &user.Passkeys[i]
		}
	}
	if pk == nil {
		return nil, ErrUnknownCredential
	}
	// The counter did not go up: another copy of this key may exist.
	if cred.Authenticator.CloneWarning {
		return &LoginResult{User: user, Passkey: pk}, ErrCloneDetected
	}

	now := time.Now().UTC()
	pk.SignCount = cred.Authenticator.SignCount
	pk.BackupState = cred.Flags.BackupState
	pk.LastUsedAt = &now
	return &LoginResult{User: user, Passkey: pk}, nil
}

func hasCredential(u *model.User, rawID []byte) bool {
	for _, p := range u.Passkeys {
		if bytes.Equal(p.CredentialID, rawID) {
			return true
		}
	}
	return false
}

// Remove deletes the passkey with id from user, reporting whether it was
// there. The caller saves the user.
func Remove(user *model.User, id string) (*model.Passkey, bool) {
	for i, p := range user.Passkeys {
		if p.Id == id {
			user.Passkeys = append(user.Passkeys[:i:i], user.Passkeys[i+1:]...)
			return &p, true
		}
	}
	return nil, false
}
