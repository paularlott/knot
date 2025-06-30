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

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
)

const (
	LEAF_PING_INTERVAL = 2 * time.Second
	RECONNECT_DELAY    = 5 * time.Second
)

func (c *Cluster) runLeafClient(originServer, originToken string) {
	log.Info().Msgf("cluster: starting link with origin server %s", originServer)

	// Remove all roles, groups, managed templates and managed template vars from the local database
	// once the initial sync is complete they will be added and up to date
	db := database.GetInstance()
	roles, err := db.GetRoles()
	if err != nil {
		log.Error().Msgf("cluster: error while getting roles from database: %s", err)
	} else {
		for _, role := range roles {
			if err := db.DeleteRole(role); err != nil {
				log.Error().Msgf("cluster: error while deleting role %s from database: %s", role.Name, err)
			}
		}
	}

	groups, err := db.GetGroups()
	if err != nil {
		log.Error().Msgf("cluster: error while getting groups from database: %s", err)
	} else {
		for _, group := range groups {
			if err := db.DeleteGroup(group); err != nil {
				log.Error().Msgf("cluster: error while deleting group %s from database: %s", group.Name, err)
			}
		}
	}

	spaces, err := db.GetSpaces()
	if err != nil {
		log.Error().Msgf("cluster: error while getting spaces from database: %s", err)
	} else {
		for _, space := range spaces {
			if space.IsDeleted {
				if err := db.DeleteSpace(space); err != nil {
					log.Error().Msgf("cluster: error while deleting space %s from database: %s", space.Name, err)
				}
			}
		}
	}

	templates, err := db.GetTemplates()
	if err != nil {
		log.Error().Msgf("cluster: error while getting templates from database: %s", err)
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
					log.Error().Msgf("cluster: error while deleting template %s from database: %s", template.Name, err)
				}
			}
		}
	}

	templateVars, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Msgf("cluster: error while getting template vars from database: %s", err)
	} else {
		for _, templateVar := range templateVars {
			if templateVar.IsManaged {
				if err := db.DeleteTemplateVar(templateVar); err != nil {
					log.Error().Msgf("cluster: error while deleting template var %s from database: %s", templateVar.Name, err)
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

			log.Debug().Msgf("cluster: connecting to %s", wsUrl)

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
					log.Fatal().Msg("cluster: failed to authenticate with origin server, check remote token")
				}

				log.Error().Msgf("cluster: error while opening websocket: %s", err)
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			log.Info().Msg("cluster: registering with origin server")

			err = leafmsg.WriteMessage(ws, leafmsg.MessageRegister, &leafmsg.Register{
				LeafVersion: build.Version,
				Zone:        cfg.Zone,
			})
			if err != nil {
				log.Error().Msgf("cluster: error while sending register message: %s", err)
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			msg, err := leafmsg.ReadMessage(ws)
			if err != nil || msg.Type != leafmsg.MessageRegister {
				log.Error().Msgf("cluster: error while reading register response: %s", err)
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}

			registerResponse := &leafmsg.RegisterResponse{}
			if err := msg.UnmarshalPayload(registerResponse); err != nil {
				log.Error().Msgf("cluster: error while unmarshalling register response: %s", err)
				ws.Close()
				time.Sleep(RECONNECT_DELAY)
				continue
			}
			if !registerResponse.Success {
				log.Fatal().Msgf("cluster: error while registering with origin server: %s", registerResponse.Error)
			}

			// Request a full sync
			if !fullSyncDone {
				err = leafmsg.WriteMessage(ws, leafmsg.MessageFullSync, nil)
				if err != nil {
					log.Error().Msgf("cluster: error while sending full sync request: %s", err)
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
						log.Error().Msgf("cluster: error while reading message from origin server: %s", err)
					} else {
						log.Info().Msg("cluster: lost connection to origin server")
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
					log.Info().Msg("cluster: leaf full sync complete")
					fullSyncDone = true

				default:
					log.Error().Msgf("cluster: unknown message type from origin %d", msg.Type)
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
		log.Error().Msgf("cluster: error while unmarshalling leaf group message: %s", err)
		return
	}

	if err := c.mergeGroups(groups); err != nil {
		log.Error().Msgf("cluster: error while merging groups from leaf: %s", err)
		return
	}
}

func (c *Cluster) handleLeafGossipRole(msg *leafmsg.Message) {
	roles := []*model.Role{}
	if err := msg.UnmarshalPayload(&roles); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling leaf role message: %s", err)
		return
	}

	if err := c.mergeRoles(roles); err != nil {
		log.Error().Msgf("cluster: error while merging roles from leaf: %s", err)
		return
	}
}

func (c *Cluster) handleLeafGossipUser(msg *leafmsg.Message) {
	users := []*model.User{}
	if err := msg.UnmarshalPayload(&users); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling leaf user message: %s", err)
		return
	}

	db := database.GetInstance()
	for _, user := range users {
		if err := db.SaveUser(user, nil); err != nil {
			log.Error().Err(err).Msgf("cluster: error while updating user %s from leaf", user.Username)
		}
	}

	// Track that we now have users so auth applies
	if len(users) > 0 {
		middleware.HasUsers = true
	}
}

func (c *Cluster) handleLeafGossipTemplate(msg *leafmsg.Message) {
	templates := []*model.Template{}
	if err := msg.UnmarshalPayload(&templates); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling leaf template message: %s", err)
		return
	}

	// Mark the templates as managed
	for _, template := range templates {
		template.IsManaged = true
	}

	if err := c.mergeTemplates(templates); err != nil {
		log.Error().Msgf("cluster: error while merging templates from leaf: %s", err)
		return
	}
}

func (c *Cluster) handleLeafGossipTemplateVar(msg *leafmsg.Message) {
	templateVars := []*model.TemplateVar{}
	if err := msg.UnmarshalPayload(&templateVars); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling leaf template var message: %s", err)
		return
	}

	// Mark the template vars as managed
	for _, templateVar := range templateVars {
		templateVar.IsManaged = true
	}

	if err := c.mergeTemplateVars(templateVars); err != nil {
		log.Error().Msgf("cluster: error while merging template vars: %s", err)
		return
	}

	// For any template vars marked as restricted delete the vars from the local database
	for _, templateVar := range templateVars {
		if templateVar.Restricted {
			database.GetInstance().DeleteTemplateVar(templateVar)
		}
	}
}
