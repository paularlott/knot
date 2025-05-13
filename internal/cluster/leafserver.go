package cluster

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/util"

	"github.com/rs/zerolog/log"
)

func (c *Cluster) HandleLeafServer(w http.ResponseWriter, r *http.Request) {

	// If there's no token then consider it an error as this end point should only be
	// used by leaf nodes using an API key
	token, ok := r.Context().Value("access_token").(*model.Token)
	if !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	user, ok := r.Context().Value("user").(*model.User)
	if !ok {
		log.Error().Msg("cluster: error while getting user from context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		log.Error().Msg("cluster: error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// Wait for the register message
	register := &leafmsg.Register{}
	msg, err := leafmsg.ReadMessage(ws)
	if err != nil || msg.Type != leafmsg.MessageRegister {
		log.Error().Msgf("cluster: error while reading message from leaf: %s", err)
		return
	}

	if err := msg.UnmarshalPayload(register); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling payload: %s", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	response := &leafmsg.RegisterResponse{
		Success: true,
		Error:   "",
	}

	// Check the version
	ourParts := strings.Split(build.Version, ".")
	versionParts := strings.Split(register.LeafVersion, ".")
	if len(ourParts) < 2 || len(versionParts) < 2 || ourParts[0] != versionParts[0] || ourParts[1] != versionParts[1] {
		log.Error().Msgf("cluster: version mismatch, our version %s, leaf version %s", build.Version, register.LeafVersion)
		response.Success = false
		response.Error = "version mismatch"
	}

	if err := leafmsg.WriteMessage(ws, leafmsg.MessageRegister, response); err != nil {
		log.Error().Msgf("cluster: error while sending leaf register response: %s", err)
		return
	}

	if !response.Success {
		return
	}

	session := c.registerLeaf(ws, user, token, register.Location)
	defer c.unregisterLeaf(session)

	log.Info().Str("location", session.Location).Msg("cluster: leaf registered")

	// Enter the message processing loop
	for {
		msg, err := leafmsg.ReadMessage(ws)
		if err != nil {
			if !strings.Contains(err.Error(), "unexpected EOF") {
				log.Error().Msgf("cluster: error while reading message from leaf: %s", err)
			} else {
				log.Info().Str("location", session.Location).Msg("cluster: leaf disconnected")
			}
			return
		}

		switch msg.Type {
		case leafmsg.MessageFullSync:
			go c.handleLeafFullSync(session)
		default:
			log.Error().Msgf("cluster: unknown message type from leaf %d", msg.Type)
		}
	}
}

func (c *Cluster) handleLeafFullSync(session *leafSession) {
	db := database.GetInstance()

	groups, err := db.GetGroups()
	if err != nil {
		log.Error().Msgf("cluster: error while getting groups: %s", err)
		return
	}
	session.SendMessage(leafmsg.MessageGossipGroup, &groups)

	roles, err := db.GetRoles()
	if err != nil {
		log.Error().Msgf("cluster: error while getting roles: %s", err)
		return
	}
	session.SendMessage(leafmsg.MessageGossipRole, &roles)

	user, err := db.GetUser(session.user.Id)
	if err != nil {
		log.Error().Msgf("cluster: error while getting user %s: %s", session.user.Username, err)
		return
	}

	users := []*model.User{user}
	session.SendMessage(leafmsg.MessageGossipUser, &users)

	templates, err := db.GetTemplates()
	if err != nil {
		log.Error().Msgf("cluster: error while getting templates: %s", err)
		return
	}
	session.SendMessage(leafmsg.MessageGossipTemplate, &templates)

	templateVars, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Msgf("cluster: error while getting template vars: %s", err)
		return
	}

	// Mask restricted template vars and trigger them to delete
	for _, templateVar := range templateVars {
		if templateVar.Restricted || templateVar.Local {
			templateVar.IsDeleted = true
			templateVar.Value = ""
			templateVar.Name = templateVar.Id
			templateVar.Location = ""
		}
	}
	session.SendMessage(leafmsg.MessageGossipTemplateVar, &templateVars)

	session.SendMessage(leafmsg.MessageFullSyncEnd, nil)
}
