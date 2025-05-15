package cluster

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/util"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	LEAF_PING_INTERVAL = 2 * time.Second
	RECONNECT_DELAY    = 5 * time.Second
)

func (c *Cluster) runLeafClient(originServer, originToken string) {
	log.Info().Msgf("cluster: starting link with origin server %s", originServer)

	go func() {
		fullSyncDone := false

		for {
			// Lookup the origin server address and make the endpoint URL
			wsUrl := strings.TrimSuffix(util.ResolveSRVHttp(originServer), "/")
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
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
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
				Location:    config.Location,
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
}

func (c *Cluster) handleLeafGossipTemplate(msg *leafmsg.Message) {
	templates := []*model.Template{}
	if err := msg.UnmarshalPayload(&templates); err != nil {
		log.Error().Msgf("cluster: error while unmarshalling leaf template message: %s", err)
		return
	}

	// Set the is managed flag on the templates so we can identify them to stop local edits
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

	if err := c.mergeTemplateVars(templateVars); err != nil {
		log.Error().Msgf("cluster: error while merging template vars from leaf: %s", err)
		return
	}

	// For any template vars marked as restricted delete the vars from the local database
	for _, templateVar := range templateVars {
		if templateVar.Restricted {
			database.GetInstance().DeleteTemplateVar(templateVar)
		}
	}
}
