package cluster

import (
	"net/http"
	"strings"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/gossip/compression"
	"github.com/paularlott/gossip/encryption"
	"github.com/paularlott/gossip/examples/common"
	"github.com/paularlott/gossip/websocket"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/database"

	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type Cluster struct {
	gossipCluster *gossip.Cluster
}

func NewCluster(clusterKey string, advertiseAddr string, bindAddr string, routes *http.ServeMux) *Cluster {
	cluster := &Cluster{}

	if viper.GetString("server.cluster.advertise_addr") != "" {

		log.Info().Msgf("cluster: enabling cluster mode on %s", viper.GetString("server.cluster.advertise_addr"))

		db := database.GetInstance()
		nodeId, err := db.GetCfgValue("node_id")
		if err != nil || nodeId.Value == "" {
			log.Fatal().Msg("server: node_id not set")
		}

		// Build configuration
		config := gossip.DefaultConfig()
		config.NodeID = nodeId.Value
		config.BindAddr = viper.GetString("server.cluster.bind_addr")
		config.AdvertiseAddr = viper.GetString("server.cluster.advertise_addr")

		if !strings.HasPrefix(config.AdvertiseAddr, "wss://") && !strings.HasPrefix(config.AdvertiseAddr, "https://") {
			config.EncryptionKey = []byte(viper.GetString("server.cluster.key"))
			config.Cipher = encryption.NewAESEncryptor()
			config.SocketTransportEnabled = true

			if viper.GetBool("server.cluster.compression") {
				config.Compressor = compression.NewSnappyCompressor()
			}
		} else {
			config.WebsocketProvider = websocket.NewGorillaProvider(5*time.Second, true, viper.GetString("server.cluster.key"))
			config.SocketTransportEnabled = false
			config.BearerToken = viper.GetString("server.cluster.key")
		}

		config.Logger = common.NewZerologLogger(log.Logger)
		config.MsgCodec = codec.NewVmihailencoMsgpackCodec()

		config.ApplicationVersion = build.Version
		config.ApplicationVersionCheck = func(version string) bool {
			ourParts := strings.Split(build.Version, ".")
			versionParts := strings.Split(build.Version, ".")
			if len(ourParts) < 2 || len(versionParts) < 2 || ourParts[0] != versionParts[0] || ourParts[1] != versionParts[1] {
				return false
			}
			return true
		}

		cluster.gossipCluster, err = gossip.NewCluster(config)
		if err != nil {
			log.Fatal().Msgf("cluster: failed to create gossip cluster: %s", err.Error())
		}

		// If using websockets then add the handler
		if config.WebsocketProvider != nil {
			routes.HandleFunc("GET /cluster", cluster.gossipCluster.WebsocketHandler)
		}

		// Add the handlers
		cluster.gossipCluster.HandleFuncWithReply(GroupFullSyncMsg, cluster.handleGroupFullSync)
		cluster.gossipCluster.HandleFunc(GroupGossipMsg, cluster.handleGroupGossip)
		cluster.gossipCluster.HandleFuncWithReply(RoleFullSyncMsg, cluster.handleRoleFullSync)
		cluster.gossipCluster.HandleFunc(RoleGossipMsg, cluster.handleRoleGossip)
		cluster.gossipCluster.HandleFuncWithReply(SpaceFullSyncMsg, cluster.handleSpaceFullSync)
		cluster.gossipCluster.HandleFunc(SpaceGossipMsg, cluster.handleSpaceGossip)
		cluster.gossipCluster.HandleFuncWithReply(TemplateFullSyncMsg, cluster.handleTemplateFullSync)
		cluster.gossipCluster.HandleFunc(TemplateGossipMsg, cluster.handleTemplateGossip)
		cluster.gossipCluster.HandleFuncWithReply(TemplateVarFullSyncMsg, cluster.handleTemplateVarFullSync)
		cluster.gossipCluster.HandleFunc(TemplateVarGossipMsg, cluster.handleTemplateVarGossip)
		cluster.gossipCluster.HandleFuncWithReply(UserFullSyncMsg, cluster.handleUserFullSync)
		cluster.gossipCluster.HandleFunc(UserGossipMsg, cluster.handleUserGossip)
		cluster.gossipCluster.HandleFuncWithReply(VolumeFullSyncMsg, cluster.handleVolumeFullSync)
		cluster.gossipCluster.HandleFunc(VolumeGossipMsg, cluster.handleVolumeGossip)

		// Periodically gossip the status of the objects
		cluster.gossipCluster.HandleGossipFunc(func() {
			cluster.gossipGroups()
			cluster.gossipRoles()
			cluster.gossipSpaces()
			cluster.gossipTemplates()
			cluster.gossipTemplateVars()
			cluster.gossipUsers()
			cluster.gossipVolumes()
		})
	}

	return cluster
}

func (c *Cluster) Start(peers []string) {
	if c.gossipCluster != nil {
		log.Info().Msg("cluster: starting gossip cluster")
		c.gossipCluster.Start()

		// Join the initial peers
		if err := c.gossipCluster.Join(peers); err != nil {
			log.Fatal().Msgf("cluster: failed to join cluster: %s", err.Error())
		}

		// If the cluster is bigger than us then trigger a full state sync
		if c.gossipCluster.NumAliveNodes() > 1 {
			go func() {
				log.Info().Msg("cluster: starting full state sync")

				nodes := c.gossipCluster.GetCandidates()

				// Try each node in order until we get a successful response
				for _, node := range nodes {
					if err := c.DoGroupFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync groups with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoRoleFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync roles with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoSpaceFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync spaces with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoTemplateFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync templates with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoTemplateVarFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync template vars with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoUserFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync users with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoVolumeFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync volumes with node %s: %s", node.ID, err.Error())
					}
				}

				log.Info().Msg("cluster: full state sync complete")
			}()
		}
	}
}

func (c *Cluster) Stop() {
	if c.gossipCluster != nil {
		log.Info().Msg("cluster: stopping gossip cluster")
		c.gossipCluster.Stop()
	}
}
