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
	LEAF_PING_INTERVAL = 15 * time.Second
)

// connect to the origin server and start processing messages from origin
func LeafConnectAndServe(server string) {
	var initialBoot = true
	var wg sync.WaitGroup
	wg.Add(1)

	origin.OriginChannel = make(chan *msg.ClientMessage, 100)

	// Start a go routine to ping the origin server
	go func() {
		for {
			time.Sleep(LEAF_PING_INTERVAL)
			pingOrigin()
		}
	}()

	go func() {
		for {

			// Lookup the origin server address and make the endpoint URL
			wsUrl := strings.TrimSuffix(util.ResolveSRVHttp(viper.GetString("server.origin_server")), "/")
			if strings.HasPrefix(wsUrl, "http") {
				wsUrl = "ws" + strings.TrimPrefix(wsUrl, "http")
			} else {
				wsUrl = "wss://" + wsUrl
			}
			wsUrl += "/api/v1/leaf-server"

			log.Debug().Msgf("leaf: connecting to %s", wsUrl)

			// Open the websocket
			header := http.Header{"Authorization": []string{fmt.Sprintf("Bearer %s", viper.GetString("server.shared_token"))}}
			dialer := websocket.DefaultDialer
			dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: viper.GetBool("tls_skip_verify")}
			dialer.HandshakeTimeout = 5 * time.Second
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
			err = msg.WriteMessage(ws, &msg.Register{
				Version:  build.Version,
				Location: viper.GetString("server.location"),
			})
			if err != nil {
				log.Error().Msgf("leaf: error while writing register message: %s", err)
				ws.Close()
				time.Sleep(3 * time.Second)
				continue
			}

			// Read the response
			var registerResponse msg.RegisterResponse
			err = msg.ReadMessage(ws, &registerResponse)
			if err != nil {
				ws.Close()

				// If versions mismatch then done
				if registerResponse.Version != build.Version {
					log.Fatal().Msg("leaf: origin and leaf servers must run the same versions.")
				}

				log.Error().Msgf("leaf: error while reading message: %s", err)
				time.Sleep(3 * time.Second)
				continue
			}

			log.Info().Msg("leaf: successfully registered with origin server")

			if registerResponse.RestrictedNode {
				server_info.RestrictedLeaf = true
				server_info.LeafLocation = registerResponse.Location
				log.Info().Msg("leaf: registered as a restricted node")
			}

			server_info.Timezone = registerResponse.Timezone
			log.Info().Msgf("leaf: origin server timezone: %s", server_info.Timezone)

			// Request origin server to sync resources
			requestTemplatesFromOrigin()
			requestTemplateVarsFromOrigin()

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

				// Get the list of spaces for the user
				spaces, err := db.GetSpacesForUser(user.Id)
				if err != nil {
					log.Error().Msgf("leaf: error fetching spaces for user %s: %s", user.Id, err)
					ws.Close()
					time.Sleep(3 * time.Second)
					break
				}

				// Request the origin server to sync the spaces
				for _, space := range spaces {
					requestSpaceFromOrigin(space.Id)
				}
			}

			// Mark the end of the bootstrap process
			bootstrapMarker()

			// Go routing to process messages from the channel
			go func() {
				for {
					message := <-origin.OriginChannel

					// Write the command
					err := msg.WriteCommand(ws, message.Command)
					if err != nil {
						log.Error().Msgf("leaf: error while writing command: %s", err)
						ws.Close()
						break
					}

					// Write the message
					if message.Payload != nil {
						err = msg.WriteMessage(ws, message.Payload)
						if err != nil {
							log.Error().Msgf("leaf: error while writing message: %s", err)
							ws.Close()
							break
						}
					}
				}
			}()

			// Run forever processing messages from the origin server
			for {
				cmd, err := msg.ReadCommand(ws)
				if err != nil {
					log.Error().Msgf("leaf: error reading command: %s", err)

					ws.Close()
					time.Sleep(3 * time.Second)
					break
				}

				switch cmd {
				case msg.MSG_BOOTSTRAP:
					if initialBoot {
						log.Debug().Msg("leaf: bootstrap")

						// Release the main app to run on the 1st run through
						initialBoot = false
						wg.Done()
					}

				case msg.MSG_PING:
					log.Debug().Msg("leaf: ping")

				case msg.MSG_UPDATE_TEMPLATE:
					err := leaf_server.HandleUpdateTemplate(ws)
					if err != nil {
						log.Error().Msgf("leaf: error updating template: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_DELETE_TEMPLATE:
					err := leaf_server.HandleDeleteTemplate(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_UPDATE_USER:
					err := leaf_server.HandleUpdateUser(ws)
					if err != nil {
						log.Error().Msgf("leaf: error updating user: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_DELETE_USER:
					err := leaf_server.HandleDeleteUser(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting user: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_UPDATE_TEMPLATEVAR:
					err := leaf_server.HandleUpdateTemplateVar(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template var: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_DELETE_TEMPLATEVAR:
					err := leaf_server.HandleDeleteTemplateVar(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting template var: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_UPDATE_SPACE:
					err := leaf_server.HandleUpdateSpace(ws)
					if err != nil {
						log.Error().Msgf("leaf: error updating space: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_DELETE_SPACE:
					err := leaf_server.HandleDeleteSpace(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting space: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				case msg.MSG_DELETE_TOKEN:
					err := leaf_server.HandleDeleteToken(ws)
					if err != nil {
						log.Error().Msgf("leaf: error deleting token: %s", err)
						ws.Close()
						time.Sleep(3 * time.Second)
						break
					}

				default:
					log.Error().Msgf("leaf: unknown command: %d", cmd)
					ws.Close()
					time.Sleep(3 * time.Second)
					break
				}
			}
		}
	}()

	// Wait for the startup to complete
	wg.Wait()
	log.Info().Msg("leaf: initialized")
}

// request the origin server to send all template hashes
func requestTemplatesFromOrigin() {
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

	message := &msg.ClientMessage{
		Command: msg.MSG_SYNC_TEMPLATES,
		Payload: syncTemplates,
	}

	origin.OriginChannel <- message
}

// request template vars from the origin server
func requestTemplateVarsFromOrigin() {
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

	message := &msg.ClientMessage{
		Command: msg.MSG_SYNC_TEMPLATEVARS,
		Payload: syncTemplateVars,
	}

	origin.OriginChannel <- message
}

// request the user update from the origin server
func requestUserFromOrigin(id string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_SYNC_USER,
		Payload: &id,
	}

	origin.OriginChannel <- message
}

func requestSpaceFromOrigin(id string) {
	message := &msg.ClientMessage{
		Command: msg.MSG_SYNC_SPACE,
		Payload: &id,
	}

	origin.OriginChannel <- message
}

func pingOrigin() {
	message := &msg.ClientMessage{
		Command: msg.MSG_PING,
		Payload: nil,
	}

	origin.OriginChannel <- message
}

func bootstrapMarker() {
	message := &msg.ClientMessage{
		Command: msg.MSG_BOOTSTRAP,
		Payload: nil,
	}

	origin.OriginChannel <- message
}
