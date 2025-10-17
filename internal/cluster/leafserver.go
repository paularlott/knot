package cluster

import (
	"net/http"
	"strings"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
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
		c.logger.Error("error while getting user from context")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Upgrade to a websocket
	ws := util.UpgradeToWS(w, r)
	if ws == nil {
		c.logger.Error("error while upgrading to websocket")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer ws.Close()

	// Wait for the register message
	register := &leafmsg.Register{}
	msg, err := leafmsg.ReadMessage(ws)
	if err != nil || msg.Type != leafmsg.MessageRegister {
		c.logger.WithError(err).Error("error while reading message from leaf:")
		return
	}

	if err := msg.UnmarshalPayload(register); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling payload:")
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
		c.logger.Error("version mismatch, our version , leaf version", "version", build.Version, "version", register.LeafVersion)
		response.Success = false
		response.Error = "version mismatch"
	}

	if err := leafmsg.WriteMessage(ws, leafmsg.MessageRegister, response); err != nil {
		c.logger.WithError(err).Error("error while sending leaf register response:")
		return
	}

	if !response.Success {
		return
	}

	session := c.registerLeaf(ws, user, token, register.Zone)
	defer c.unregisterLeaf(session)

	c.logger.Info("leaf registered", "zone", session.Zone)

	// Enter the message processing loop
	for {
		msg, err := leafmsg.ReadMessage(ws)
		if err != nil {
			if !strings.Contains(err.Error(), "unexpected EOF") {
				c.logger.WithError(err).Error("error while reading message from leaf:")
			} else {
				c.logger.Info("leaf disconnected", "zone", session.Zone)
			}
			return
		}

		switch msg.Type {
		case leafmsg.MessageFullSync:
			go c.handleLeafFullSync(session)
		default:
			c.logger.Error("unknown message type from leaf", "type", msg.Type)
		}
	}
}

func (c *Cluster) handleLeafFullSync(session *leafSession) {
	db := database.GetInstance()

	groups, err := db.GetGroups()
	if err != nil {
		c.logger.WithError(err).Error("error while getting groups:")
		return
	}
	session.SendMessage(leafmsg.MessageGossipGroup, &groups)

	roles, err := db.GetRoles()
	if err != nil {
		c.logger.WithError(err).Error("error while getting roles:")
		return
	}
	session.SendMessage(leafmsg.MessageGossipRole, &roles)

	user, err := db.GetUser(session.user.Id)
	if err != nil {
		c.logger.Error("error while getting user :", "username", session.user.Username)
		return
	}

	users := []*model.User{user}
	session.SendMessage(leafmsg.MessageGossipUser, &users)

	templates, err := db.GetTemplates()
	if err != nil {
		c.logger.WithError(err).Error("error while getting templates:")
		return
	}
	session.SendMessage(leafmsg.MessageGossipTemplate, &templates)

	templateVars, err := db.GetTemplateVars()
	if err != nil {
		c.logger.WithError(err).Error("error while getting template vars:")
		return
	}

	// Mask restricted template vars and trigger them to delete
	for _, templateVar := range templateVars {
		// Only allow vars that have empty zones or explicitly mention leaf node zone
		allowVar := len(templateVar.Zones) == 0
		for _, zone := range templateVar.Zones {
			if zone == model.LeafNodeZone {
				allowVar = true
				break
			}
		}

		if templateVar.Restricted || templateVar.Local || !allowVar {
			templateVar.IsDeleted = true
			templateVar.Value = ""
			templateVar.Name = templateVar.Id
			templateVar.Zones = []string{}
		}
	}
	session.SendMessage(leafmsg.MessageGossipTemplateVar, &templateVars)

	session.SendMessage(leafmsg.MessageFullSyncEnd, nil)
}
