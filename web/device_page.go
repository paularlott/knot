package web

import (
	"fmt"
	"net/http"

	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/devicegrant"
	"github.com/paularlott/knot/internal/service"
	"github.com/paularlott/knot/internal/sse"
	"github.com/paularlott/knot/internal/util/audit"

	"github.com/paularlott/knot/internal/log"
)

// HandleDevicePage is GET /device: the form for the code `knot connect`
// shows, and with ?user_code= (typed, or from the link the CLI opened) the
// confirmation that approves it.
func HandleDevicePage(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)

	typed := r.URL.Query().Get("user_code")
	data["userCode"] = typed
	if typed == "" {
		renderDevicePage(w, http.StatusOK, data)
		return
	}

	grant, limited := devicegrant.ByUserCode(user.Id, typed)
	switch {
	case limited:
		data["error"] = "Too many codes were tried. Wait 15 minutes, then try again."
		renderDevicePage(w, http.StatusTooManyRequests, data)
	case grant == nil:
		data["error"] = "That code isn't valid. It may have expired: check the code shown by knot connect, or run it again."
		renderDevicePage(w, http.StatusBadRequest, data)
	default:
		data["userCode"] = devicegrant.FormatUserCode(grant.UserCode)
		data["tokenName"] = grant.TokenName
		data["confirm"] = true
		renderDevicePage(w, http.StatusOK, data)
	}
}

// HandleDeviceDecision is POST /device: the user approves or denies the
// device sign-in of a user code. Approving makes the API token, which the
// CLI then collects from the token endpoint.
func HandleDeviceDecision(w http.ResponseWriter, r *http.Request) {
	user, data := getCommonTemplateData(r)

	grant, limited := devicegrant.ByUserCode(user.Id, r.FormValue("user_code"))
	if limited || grant == nil {
		data["error"] = "That code isn't valid any more. Run knot connect again to get a new one."
		renderDevicePage(w, http.StatusBadRequest, data)
		return
	}

	if r.FormValue("action") != "approve" {
		grant.Status = devicegrant.StatusDenied
		devicegrant.Apply(grant)
		service.GetTransport().GossipDeviceGrant(grant)

		data["done"] = "denied"
		renderDevicePage(w, http.StatusOK, data)
		return
	}

	token := model.NewToken(grant.TokenName, user.Id)
	token.Source = model.TokenSourceDevice
	if err := database.GetInstance().SaveToken(token); err != nil {
		log.WithError(err).Error("failed to save device sign-in token")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	service.GetTransport().GossipToken(token)
	sse.PublishTokensChanged("")

	audit.LogWithRequest(r,
		user.Username,
		model.AuditActorTypeUser,
		model.AuditEventTokenCreate,
		fmt.Sprintf("Created token %s by approving a device sign-in", token.Name),
		&map[string]interface{}{
			"token_id":   token.Id,
			"token_name": token.Name,
		},
	)

	grant.Status = devicegrant.StatusApproved
	grant.UserId = user.Id
	grant.TokenId = token.Id
	devicegrant.Apply(grant)
	service.GetTransport().GossipDeviceGrant(grant)

	data["done"] = "approved"
	renderDevicePage(w, http.StatusOK, data)
}

func renderDevicePage(w http.ResponseWriter, status int, data map[string]interface{}) {
	tmpl, err := newTemplate("device.tmpl")
	if err != nil {
		log.Error(err.Error())
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if _, ok := data["done"]; !ok {
		data["done"] = ""
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if err := tmpl.Execute(w, data); err != nil {
		log.Error(err.Error())
	}
}
