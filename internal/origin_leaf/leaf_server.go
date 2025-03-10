package origin_leaf

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"
	"github.com/paularlott/knot/internal/origin_leaf/leaf_server"
	"github.com/paularlott/knot/internal/origin_leaf/msg"
	"github.com/paularlott/knot/internal/origin_leaf/origin"
	"github.com/paularlott/knot/internal/origin_leaf/server_info"
	"github.com/paularlott/knot/util"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	LEAF_PING_INTERVAL = 2 * time.Second
	RECONNECT_DELAY    = 2 * time.Second
)

// connect to the origin server and start processing messages from origin
func LeafConnectAndServe(server string) {
	var initialBoot = true
	var wg sync.WaitGroup
	wg.Add(1)

	origin.OriginChannel = make(chan *msg.LeafOriginMessage, 100)
	origin.OriginRetryChannel = make(chan *msg.LeafOriginMessage, 2)

	go func() {

		// Track the session ID with the origin server used during a reconnect
		leafSessionId := ""

		var doFullSync bool
		lastFullSyncCompleted := false

		for {

			// Lookup the origin server address and make the endpoint URL
			wsUrl := strings.TrimSuffix(util.ResolveSRVHttp(viper.GetString("server.origin_server")), "/")
			if strings.HasPrefix(wsUrl, "http") {
				wsUrl = "ws" + strings.TrimPrefix(wsUrl, "http")
			} else {
				wsUrl = "wss://" + wsUrl
			}
			wsUrl += "/api/leaf-server"

			log.Debug().Msgf("leaf: connecting to %s", wsUrl)

			// Open the websocket
			header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", viper.GetString("server.shared_token"))}}
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
			dialer.HandshakeTimeout = 10 * time.Second
			dialer.ReadBufferSize = 32768
			dialer.WriteBufferSize = 32768
			dialer.EnableCompression = true
			ws, response, err := dialer.Dial(wsUrl, header)
			if err != nil {
				if response != nil && response.StatusCode == http.StatusUnauthorized {
					log.Fatal().Msg("leaf: failed to authenticate with origin server, check remote token")
				}

				log.Error().Msgf("leaf: error while opening websocket: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}

			log.Debug().Msg("leaf: registering with origin server")

			// Write a register message to the origin server
			err = msg.WriteMessage(ws, msg.MSG_REGISTER, &msg.Register{
				Version:   build.Version,
				Location:  viper.GetString("server.location"),
				SessionId: leafSessionId,
			})
			if err != nil {
				log.Error().Msgf("leaf: error while writing register message: %s", err)
				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Read the response
			message, err := msg.ReadMessgae(ws)
			if err != nil {
				log.Error().Msgf("leaf: error while reading register response: %s", err)

				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			var registerResponse msg.RegisterResponse
			err = message.UnmarshalPayload(&registerResponse)
			if err != nil {
				log.Error().Msgf("leaf: error while reading message: %s", err)

				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// If versions mismatch then done
			originVersionParts := strings.Split(registerResponse.Version, ".")
			leafVersionParts := strings.Split(build.Version, ".")
			if len(originVersionParts) < 2 || len(leafVersionParts) < 2 || originVersionParts[0] != leafVersionParts[0] || originVersionParts[1] != leafVersionParts[1] {
				ws.Close()
				log.Fatal().Str("origin version", registerResponse.Version).Str("leaf version", build.Version).Msg("leaf: origin and leaf servers must run the same major and minor versions.")
			}

			// Save the session ID for the next connection
			doFullSync = leafSessionId != registerResponse.SessionId || !lastFullSyncCompleted
			leafSessionId = registerResponse.SessionId

			log.Info().Msg("leaf: successfully registered with origin server")

			if registerResponse.RestrictedNode {
				server_info.RestrictedLeaf = true
				server_info.LeafLocation = registerResponse.Location
				log.Info().Msg("leaf: registered as a restricted node")
			}

			server_info.Timezone = registerResponse.Timezone
			log.Info().Msgf("leaf: origin server timezone: %s", server_info.Timezone)

			// Send ping messages at regular intervals
			go func() {
				for {
					time.Sleep(LEAF_PING_INTERVAL)
					err := ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
					if err != nil {
						log.Error().Msgf("leaf: error sending ping: %s", err)

						ws.Close()
						break
					}
				}
			}()

			if doFullSync {
				lastFullSyncCompleted = false

				// Request origin server to sync resources
				requestTemplatesFromOrigin()
				requestTemplateVarsFromOrigin()
				requestRolesFromOrigin()

				// Request sync from the origin server
				db := database.GetInstance()

				log.Info().Msg("leaf: requesting sync of users from origin server")
				users, err := db.GetUsers()
				if err != nil {
					log.Error().Msgf("leaf: error fetching users: %s", err)
					ws.Close()
					time.Sleep(3 * time.Second)
					continue
				}

				// Request the local users be updated from the origin server
				for _, user := range users {
					requestUserFromOrigin(user.Id)

					// Get the list of spaces for the user in the local database
					spaces, err := db.GetSpacesForUser(user.Id)
					if err != nil {
						log.Error().Msgf("leaf: error fetching spaces for user %s: %s", user.Id, err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

					// Request the origin server to sync the spaces, this will update the local data and remove local spaces that don't exist
					var ids []string
					for _, space := range spaces {
						requestSpaceFromOrigin(space.Id)
						ids = append(ids, space.Id)
					}

					// Ask the origin server to send all spaces for the user other than those already synced
					RequestUserSpacesFromOrigin(user.Id, ids)
				}

				// Mark the end of the bootstrap process
				bootstrapMarker()
			}

			// Send the next message form the channel to the origin server
			go func() {
				for {
					var isRetry bool
					var message *msg.LeafOriginMessage

					select {
					case message = <-origin.OriginRetryChannel:
						isRetry = true

					case message = <-origin.OriginChannel:
						isRetry = false
					}

					// Write the message
					err := msg.WriteMessage(ws, message.Command, message.Payload)
					if err != nil {
						// If not retry then close the connection and put the message on the retry channel
						if !isRetry {
							log.Warn().Msgf("leaf: error writing message to origin server: %s", err)
							origin.OriginRetryChannel <- message
						} else {
							log.Error().Msgf("leaf: error writing message to origin server: %s", err)
						}

						ws.Close()
						break
					}
				}
			}()

			// Run forever processing messages from the origin server
			for {
				message, err := msg.ReadMessgae(ws)
				if err != nil {
					log.Error().Msgf("leaf: error while reading message: %s", err)
					ws.Close()

					break
				}

				switch message.Command {
				case msg.MSG_BOOTSTRAP:
					if initialBoot {
						log.Debug().Msg("leaf: bootstrap done")

						// Release the main app to run on the 1st run through
						initialBoot = false
						wg.Done()
					}

					lastFullSyncCompleted = true

				case msg.MSG_UPDATE_TEMPLATE:
					err = leaf_server.HandleUpdateTemplate(message)
					if err != nil {
						log.Error().Msgf("leaf: error updating template: %s", err)
					}

				case msg.MSG_DELETE_TEMPLATE:
					err = leaf_server.HandleDeleteTemplate(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template: %s", err)
					}

				case msg.MSG_UPDATE_USER:
					err = leaf_server.HandleUpdateUser(message)
					if err != nil {
						log.Error().Msgf("leaf: error updating user: %s", err)
					}

				case msg.MSG_DELETE_USER:
					err = leaf_server.HandleDeleteUser(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting user: %s", err)
					}

				case msg.MSG_UPDATE_TEMPLATEVAR:
					err = leaf_server.HandleUpdateTemplateVar(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template var: %s", err)
					}

				case msg.MSG_DELETE_TEMPLATEVAR:
					err = leaf_server.HandleDeleteTemplateVar(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template var: %s", err)
					}

				case msg.MSG_UPDATE_SPACE:
					err = leaf_server.HandleUpdateSpace(message)
					if err != nil {
						log.Error().Msgf("leaf: error updating space: %s", err)
					}

				case msg.MSG_DELETE_SPACE:
					err = leaf_server.HandleDeleteSpace(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting space: %s", err)
					}

				case msg.MSG_DELETE_TOKEN:
					err = leaf_server.HandleDeleteToken(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting token: %s", err)
					}

				case msg.MSG_UPDATE_ROLE:
					err = leaf_server.HandleUpdateRole(message)
					if err != nil {
						log.Error().Msgf("leaf: error updating role: %s", err)
					}

				case msg.MSG_DELETE_ROLE:
					err = leaf_server.HandleDeleteRole(message)
					if err != nil {
						log.Error().Msgf("leaf: error deleting role: %s", err)
					}

				default:
					log.Error().Msgf("leaf: unknown command: %d", message.Command)
					err = fmt.Errorf("unknown command: %d", message.Command)
				}

				if err != nil {
					ws.Close()
					break
				}
			}

			// Wait a bit before trying to reconnect
			time.Sleep(RECONNECT_DELAY)
		}
	}()

	// Wait for the startup to complete
	wg.Wait()
	log.Info().Msg("leaf: initialized")
}

// request the origin server to send all template hashes
func requestTemplatesFromOrigin() {
	log.Info().Msg("leaf: requesting sync of templates from origin server")

	syncTemplates := &msg.SyncTemplates{
		Existing: []string{},
	}

	// load the existing templates from the database & send to the origin server
	db := database.GetInstance()
	templates, err := db.GetTemplates()
	if err != nil {
		log.Error().Msgf("error fetching templates: %s", err)
		return
	}

	for _, template := range templates {
		syncTemplates.Existing = append(syncTemplates.Existing, template.Id)
	}

	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_TEMPLATES,
		Payload: syncTemplates,
	}

	origin.OriginChannel <- message
}

// request template vars from the origin server
func requestTemplateVarsFromOrigin() {
	log.Info().Msg("leaf: requesting sync of variables from origin server")

	syncTemplateVars := &msg.SyncTemplateVars{
		Existing: []string{},
	}

	// Load the existing template vars from thr database & send to the origin server
	db := database.GetInstance()
	templateVars, err := db.GetTemplateVars()
	if err != nil {
		log.Error().Msgf("error fetching template vars: %s", err)
		return
	}

	for _, templateVar := range templateVars {
		if !templateVar.Local {
			syncTemplateVars.Existing = append(syncTemplateVars.Existing, templateVar.Id)
		}
	}

	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_TEMPLATEVARS,
		Payload: syncTemplateVars,
	}

	origin.OriginChannel <- message
}

// request the user update from the origin server
func requestUserFromOrigin(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_USER,
		Payload: &id,
	}

	origin.OriginChannel <- message
}

func requestSpaceFromOrigin(id string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_SPACE,
		Payload: &id,
	}

	origin.OriginChannel <- message
}

func RequestUserSpacesFromOrigin(userId string, existing []string) {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_USER_SPACES,
		Payload: &msg.SyncUserSpaces{
			UserId:   userId,
			Existing: existing,
		},
	}

	origin.OriginChannel <- message
}

func bootstrapMarker() {
	message := &msg.LeafOriginMessage{
		Command: msg.MSG_BOOTSTRAP,
		Payload: nil,
	}

	origin.OriginChannel <- message
}

func requestRolesFromOrigin() {
	log.Info().Msg("leaf: requesting sync of roles from origin server")

	message := &msg.LeafOriginMessage{
		Command: msg.MSG_SYNC_ROLES,
		Payload: nil,
	}

	origin.OriginChannel <- message
}
