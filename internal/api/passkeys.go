package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/knot/apiclient"
	"github.com/paularlott/knot/internal/authratelimit"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/log"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/passkey"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"
	"github.com/paularlott/knot/internal/util/rest"
)

// maxPasskeyBody bounds a WebAuthn response; real ones are a few KB.
const maxPasskeyBody = 64 * 1024

// PasskeyRegisterFinishRequest is the body of POST /api/users/whoami/passkeys/finish.
type PasskeyRegisterFinishRequest struct {
	Name       string          `json:"name"`
	Credential json.RawMessage `json:"credential"`
}

// passkeySignInRefused reports why user may not sign in with, or add, a
// passkey, or "" when they may. Pro replaces it so an account that must
// sign in through its provider can't get round it with a passkey.
var passkeySignInRefused = func(user *model.User) string { return "" }

// passkeyRateKey is the rate limiter's account key for passkey sign-ins,
// which name no account until they verify: the client's address stands in.
func passkeyRateKey(clientIP string) string {
	return "passkey:" + clientIP
}

// HandlePasskeyLoginBegin starts a passkey sign-in and returns the options
// for navigator.credentials.get().
func HandlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	clientIP := authClientIP(r)
	if config.GetServerConfig().AuthIPRateLimiting && authratelimit.Blocked(clientIP, passkeyRateKey(clientIP)) {
		rest.WriteResponse(http.StatusTooManyRequests, w, r, ErrorResponse{Error: "too many requests"})
		return
	}

	options, err := passkey.BeginLogin(w, r)
	if err != nil {
		writePasskeyError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, options)
}

// HandlePasskeyLoginFinish verifies the passkey sign-in and starts a web
// session for its user.
func HandlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	cfg := config.GetServerConfig()
	clientIP := authClientIP(r)
	rateKey := passkeyRateKey(clientIP)
	if cfg.AuthIPRateLimiting && authratelimit.Blocked(clientIP, rateKey) {
		rest.WriteResponse(http.StatusTooManyRequests, w, r, ErrorResponse{Error: "too many requests"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxPasskeyBody))
	if err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid request"})
		return
	}

	result, err := passkey.FinishLogin(w, r, body)
	if err != nil {
		if cfg.AuthIPRateLimiting && !errors.Is(err, passkey.ErrChallenge) {
			until, _ := authratelimit.RecordFailure(clientIP, rateKey)
			gossipAuthFailure(&authratelimit.Event{IP: clientIP, Email: rateKey, At: time.Now(), BlockUntil: until})
		}
		actor := "unknown"
		details := map[string]interface{}{"provider": "passkey", "reason": err.Error()}
		if result != nil {
			actor = result.User.Username
			details["passkey_id"] = result.Passkey.Id
			details["passkey_name"] = result.Passkey.Name
		}
		audit.LogWithRequest(r, actor, model.AuditActorTypeUser, model.AuditEventAuthFailed, "Passkey sign-in failed", &details)
		writePasskeyError(w, r, err)
		return
	}

	user := result.User
	if reason := passkeySignInRefused(user); reason != "" {
		audit.LogWithRequest(r,
			user.Username,
			model.AuditActorTypeUser,
			model.AuditEventAuthFailed,
			"Passkey sign-in refused: "+reason,
			&map[string]interface{}{"provider": "passkey", "passkey_id": result.Passkey.Id},
		)
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: reason})
		return
	}

	now := time.Now().UTC()
	user.LastLoginAt = &now
	user.UpdatedAt = hlc.Now()
	if err := database.GetInstance().SaveUser(user, []string{"Passkeys", "LastLoginAt", "UpdatedAt"}); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipUser(user)

	session := model.NewSession(r, user.Id)
	if err := database.GetSessionStorage().SaveSession(session); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipSession(session)
	sse.PublishSessionsChanged("")

	middleware.DeleteSessionCookie(w, r)
	middleware.SetCookie(w, r, model.WebSessionCookie, session.Id, 0)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventAuthOk,
		"",
		&map[string]interface{}{
			"email":        user.Email,
			"provider":     "passkey",
			"passkey_id":   result.Passkey.Id,
			"passkey_name": result.Passkey.Name,
		},
	)

	authratelimit.Clear(clientIP, rateKey)
	gossipAuthFailure(&authratelimit.Event{IP: clientIP, Email: rateKey, At: time.Now(), Clear: true})

	rest.WriteResponse(http.StatusOK, w, r, apiclient.AuthLoginResponse{Status: true})
}

// passkeyRegistrant returns the signed-in user when they may add a passkey
// to their own account: in a browser session, not switched to another user
// (whoever switched would be adding their own authenticator to it).
func passkeyRegistrant(w http.ResponseWriter, r *http.Request) *model.User {
	user := r.Context().Value("user").(*model.User)
	session, _ := r.Context().Value("session").(*model.Session)
	if session == nil {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "passkeys can only be added from the web interface"})
		return nil
	}
	if session.OriginalUserId != "" && session.OriginalUserId != user.Id {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: "switch back to your own account to add a passkey"})
		return nil
	}
	if reason := passkeySignInRefused(user); reason != "" {
		rest.WriteResponse(http.StatusForbidden, w, r, ErrorResponse{Error: reason})
		return nil
	}
	return user
}

// HandlePasskeyRegisterBegin starts adding a passkey to the signed-in user
// and returns the options for navigator.credentials.create().
func HandlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	user := passkeyRegistrant(w, r)
	if user == nil {
		return
	}
	options, err := passkey.BeginRegistration(w, r, user)
	if err != nil {
		writePasskeyError(w, r, err)
		return
	}
	rest.WriteResponse(http.StatusOK, w, r, options)
}

// HandlePasskeyRegisterFinish verifies the new passkey and stores it on the
// signed-in user.
func HandlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	user := passkeyRegistrant(w, r)
	if user == nil {
		return
	}

	request := PasskeyRegisterFinishRequest{}
	if err := json.NewDecoder(io.LimitReader(r.Body, maxPasskeyBody)).Decode(&request); err != nil {
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: "invalid request"})
		return
	}

	// Read the user again so the passkey is added to the stored list.
	db := database.GetInstance()
	fresh, err := db.GetUser(user.Id)
	if err != nil || fresh == nil {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	pk, err := passkey.FinishRegistration(w, r, fresh, request.Credential, request.Name)
	if err != nil {
		writePasskeyError(w, r, err)
		return
	}

	fresh.UpdatedAt = hlc.Now()
	if err := db.SaveUser(fresh, []string{"Passkeys", "UpdatedAt"}); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipUser(fresh)
	sse.PublishUsersChanged(fresh.Id)

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventPasskeyAdd,
		fmt.Sprintf("Added passkey %s", pk.Name),
		&map[string]interface{}{
			"user_id":      fresh.Id,
			"passkey_id":   pk.Id,
			"passkey_name": pk.Name,
		},
	)

	rest.WriteResponse(http.StatusCreated, w, r, passkeyInfo(pk))
}

// HandleGetUserPasskeys lists a user's passkeys, without their key material.
func HandleGetUserPasskeys(w http.ResponseWriter, r *http.Request) {
	user, err := database.GetInstance().GetUser(r.PathValue("user_id"))
	if err != nil || user == nil || user.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	list := apiclient.PasskeyList{Passkeys: make([]apiclient.PasskeyInfo, 0, len(user.Passkeys))}
	for i := range user.Passkeys {
		list.Passkeys = append(list.Passkeys, passkeyInfo(&user.Passkeys[i]))
	}
	list.Count = len(list.Passkeys)
	rest.WriteResponse(http.StatusOK, w, r, list)
}

// HandleDeleteUserPasskey removes one of a user's passkeys.
func HandleDeleteUserPasskey(w http.ResponseWriter, r *http.Request) {
	actor := r.Context().Value("user").(*model.User)
	db := database.GetInstance()

	user, err := db.GetUser(r.PathValue("user_id"))
	if err != nil || user == nil || user.IsDeleted {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "user not found"})
		return
	}

	pk, ok := passkey.Remove(user, r.PathValue("passkey_id"))
	if !ok {
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: "passkey not found"})
		return
	}

	user.UpdatedAt = hlc.Now()
	if err := db.SaveUser(user, []string{"Passkeys", "UpdatedAt"}); err != nil {
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: err.Error()})
		return
	}
	service.GetTransport().GossipUser(user)
	sse.PublishUsersChanged(user.Id)

	audit.LogWithRequest(r,
		actor.Username,
		model.AuditActorTypeUser,
		model.AuditEventPasskeyDelete,
		fmt.Sprintf("Removed passkey %s from %s", pk.Name, user.Username),
		&map[string]interface{}{
			"user_id":      user.Id,
			"passkey_id":   pk.Id,
			"passkey_name": pk.Name,
		},
	)

	w.WriteHeader(http.StatusOK)
}

func passkeyInfo(pk *model.Passkey) apiclient.PasskeyInfo {
	return apiclient.PasskeyInfo{
		Id:             pk.Id,
		Name:           pk.Name,
		BackupEligible: pk.BackupEligible,
		BackupState:    pk.BackupState,
		CreatedAt:      pk.CreatedAt,
		LastUsedAt:     pk.LastUsedAt,
	}
}

// writePasskeyError answers with the status for a passkey error. The
// messages are fixed, so they are safe to show.
func writePasskeyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, passkey.ErrDisabled):
		rest.WriteResponse(http.StatusNotFound, w, r, ErrorResponse{Error: passkey.ErrDisabled.Error()})
	case errors.Is(err, passkey.ErrChallenge):
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: passkey.ErrChallenge.Error()})
	case errors.Is(err, passkey.ErrTooMany):
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: passkey.ErrTooMany.Error()})
	case errors.Is(err, passkey.ErrUnknownCredential), errors.Is(err, passkey.ErrInactive), errors.Is(err, passkey.ErrCloneDetected):
		// One answer, so a probe can't tell an unknown passkey from a disabled account.
		rest.WriteResponse(http.StatusUnauthorized, w, r, ErrorResponse{Error: "that passkey can't sign in to this server"})
	case errors.Is(err, passkey.ErrVerify):
		log.Debug("passkey verification failed", "error", err)
		rest.WriteResponse(http.StatusBadRequest, w, r, ErrorResponse{Error: err.Error()})
	default:
		log.WithError(err).Error("passkey")
		rest.WriteResponse(http.StatusInternalServerError, w, r, ErrorResponse{Error: "passkey request failed"})
	}
}
