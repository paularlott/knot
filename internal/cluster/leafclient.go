package cluster

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/sse"

	"github.com/gorilla/websocket"
)

const (
	LEAF_PING_INTERVAL = 2 * time.Second
	RECONNECT_DELAY    = 5 * time.Second
)

func (c *Cluster) runLeafClient(originServer, originToken string) {
	c.logger.Info("starting link with origin server", "originServer", originServer)

	// Remove all roles, groups, managed templates and managed template vars from the local database
	// once the initial sync is complete they will be added and up to date
	db := database.GetInstance()
	roles, err := db.GetRoles()
	if err != nil {
		c.logger.WithError(err).Error("error while getting roles from database:")
	} else {
		for _, role := range roles {
			if err := db.DeleteRole(role); err != nil {
				c.logger.Error("error while deleting role  from database:", "cluster", role.Name)
			}
		}
	}

	groups, err := db.GetGroups()
	if err != nil {
		c.logger.WithError(err).Error("error while getting groups from database:")
	} else {
		for _, group := range groups {
			if err := db.DeleteGroup(group); err != nil {
				c.logger.Error("error while deleting group  from database:", "cluster", group.Name)
			}
		}
	}

	spaces, err := db.GetSpaces()
	if err != nil {
		c.logger.WithError(err).Error("error while getting spaces from database:")
	} else {
		for _, space := range spaces {
			if space.IsDeleted {
				if err := db.DeleteSpace(space); err != nil {
					c.logger.Error("error while deleting space  from database:", "space_name", space.Name)
				}
			}
		}
	}

	templates, err := db.GetTemplates()
	if err != nil {
		c.logger.WithError(err).Error("error while getting templates from database:")
	} else {
		for _, template := range templates {
			if template.IsManaged {
				spaces, err := db.GetSpacesByTemplateId(template.Id)
				if err == nil && len(spaces) > 0 {
					// Check if any of the spaces are not marked for deletion
					for _, space := range spaces {
						if !space.IsDeleted {
							continue
						}
					}
				}

				if err := db.DeleteTemplate(template); err != nil {
					c.logger.Error("error while deleting template  from database:", "template", template.Name)
				}
			}
		}
	}

	templateVars, err := db.GetTemplateVars()
	if err != nil {
		c.logger.WithError(err).Error("error while getting template vars from database:")
	} else {
		for _, templateVar := range templateVars {
			if templateVar.IsManaged {
				if err := db.DeleteTemplateVar(templateVar); err != nil {
					c.logger.Error("error while deleting template var  from database:", "template", templateVar.Name)
				}
			}
		}
	}

	go func() {
		fullSyncDone := false

		cfg := config.GetServerConfig()
		for {
			// Lookup the origin server address and make the endpoint URL
			wsUrl := strings.TrimSuffix(dns.ResolveSRVHttp(originServer), "/")
			if strings.HasPrefix(wsUrl, "http") {
				wsUrl = "ws" + strings.TrimPrefix(wsUrl, "http")
			} else {
				wsUrl = "wss://" + wsUrl
			}
			wsUrl += "/cluster/leaf"

			c.logger.Debug("connecting to", "wsUrl", wsUrl)

			// Open the websocket
			header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", originToken)}}
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: cfg.TLS.SkipVerify}
			dialer.HandshakeTimeout = 10 * time.Second
			dialer.ReadBufferSize = 32768
			dialer.WriteBufferSize = 32768
			dialer.EnableCompression = true
			ws, response, err := dialer.Dial(wsUrl, header)
			if err != nil {
				if response != nil && response.StatusCode == http.StatusUnauthorized {
					c.logger.Fatal("failed to authenticate with origin server, check remote token")
				}

				c.logger.WithError(err).Error("error while opening websocket:")
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			c.logger.Info("registering with origin server")

			err = leafmsg.WriteMessage(ws, leafmsg.MessageRegister, &leafmsg.Register{
				LeafVersion: build.Version,
				Zone:        cfg.Zone,
			})
			if err != nil {
				c.logger.WithError(err).Error("error while sending register message:")
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			msg, err := leafmsg.ReadMessage(ws)
			if err != nil || msg.Type != leafmsg.MessageRegister {
				c.logger.WithError(err).Error("error while reading register response:")
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			registerResponse := &leafmsg.RegisterResponse{}
			if err := msg.UnmarshalPayload(registerResponse); err != nil {
				c.logger.WithError(err).Error("error while unmarshalling register response:")
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}
			if !registerResponse.Success {
				c.logger.Fatal("error while registering with origin server", "error", registerResponse.Error)
			}

			// Request a full sync
			if !fullSyncDone {
				err = leafmsg.WriteMessage(ws, leafmsg.MessageFullSync, nil)
				if err != nil {
					c.logger.WithError(err).Error("error while sending full sync request:")
					ws.Close()
					time.Sleep(RECONNECT_DELAY)
					continue
				}
			}

			// Enter the message processing loop
			for {
				msg, err := leafmsg.ReadMessage(ws)
				if err != nil {
					if !strings.Contains(err.Error(), "unexpected EOF") {
						c.logger.WithError(err).Error("error while reading message from origin server:")
					} else {
						c.logger.Info("lost connection to origin server")
					}
					break
				}

				switch msg.Type {
				case leafmsg.MessageGossipGroup:
					c.handleLeafGossipGroup(msg)

				case leafmsg.MessageGossipRole:
					c.handleLeafGossipRole(msg)

				case leafmsg.MessageGossipUser:
					c.handleLeafGossipUser(msg)

				case leafmsg.MessageGossipTemplate:
					c.handleLeafGossipTemplate(msg)

				case leafmsg.MessageGossipTemplateVar:
					c.handleLeafGossipTemplateVar(msg)

				case leafmsg.MessageFullSyncEnd:
					c.logger.Info("leaf full sync complete")
					fullSyncDone = true

				default:
					c.logger.Error("unknown message type from origin", "type", msg.Type)
				}
			}

			// Wait before trying to reconnect
			ws.Close()
			time.Sleep(RECONNECT_DELAY)
		}
	}()
}

func (c *Cluster) handleLeafGossipGroup(msg *leafmsg.Message) {
	groups := []*model.Group{}
	if err := msg.UnmarshalPayload(&groups); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling leaf group message:")
		return
	}

	if err := c.mergeGroups(groups); err != nil {
		c.logger.WithError(err).Error("error while merging groups from leaf:")
		return
	}
}

func (c *Cluster) handleLeafGossipRole(msg *leafmsg.Message) {
	roles := []*model.Role{}
	if err := msg.UnmarshalPayload(&roles); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling leaf role message:")
		return
	}

	if err := c.mergeRoles(roles); err != nil {
		c.logger.WithError(err).Error("error while merging roles from leaf:")
		return
	}
}

func (c *Cluster) handleLeafGossipUser(msg *leafmsg.Message) {
	users := []*model.User{}
	if err := msg.UnmarshalPayload(&users); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling leaf user message:")
		return
	}

	db := database.GetInstance()
	for _, user := range users {
		if err := db.SaveUser(user, nil); err != nil {
			c.logger.Error("error while updating user  from leaf", "username", user.Username)
		}
	}

	// Track that we now have users so auth applies
	if len(users) > 0 {
		middleware.HasUsers = true
	}

	// Notify SSE clients of user changes
	sse.PublishUsersChanged()
}

func (c *Cluster) handleLeafGossipTemplate(msg *leafmsg.Message) {
	templates := []*model.Template{}
	if err := msg.UnmarshalPayload(&templates); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling leaf template message:")
		return
	}

	// Mark the templates as managed
	for _, template := range templates {
		template.IsManaged = true
	}

	if err := c.mergeTemplates(templates); err != nil {
		c.logger.WithError(err).Error("error while merging templates from leaf:")
		return
	}
}

func (c *Cluster) handleLeafGossipTemplateVar(msg *leafmsg.Message) {
	templateVars := []*model.TemplateVar{}
	if err := msg.UnmarshalPayload(&templateVars); err != nil {
		c.logger.WithError(err).Error("error while unmarshalling leaf template var message:")
		return
	}

	// Mark the template vars as managed
	for _, templateVar := range templateVars {
		templateVar.IsManaged = true
	}

	if err := c.mergeTemplateVars(templateVars); err != nil {
		c.logger.WithError(err).Error("error while merging template vars:")
		return
	}

	// For any template vars marked as restricted delete the vars from the local database
	for _, templateVar := range templateVars {
		if templateVar.Restricted {
			database.GetInstance().DeleteTemplateVar(templateVar)
		}
	}
}
