package cluster

import (
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/gossip/compression"
	"github.com/paularlott/gossip/encryption"
	"github.com/paularlott/gossip/examples/common"
	"github.com/paularlott/gossip/websocket"
	"github.com/paularlott/knot/build"
	cfg "github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/middleware"

	"github.com/rs/zerolog/log"
)

const (
	GossipInterval = 15 * time.Second
)

type Cluster struct {
	gossipCluster  *gossip.Cluster
	config         *gossip.Config
	leafSessionMux sync.RWMutex
	leafSessions   map[uuid.UUID]*leafSession
}

func NewCluster(clusterKey string, advertiseAddr string, bindAddr string, routes *http.ServeMux, compress bool) *Cluster {
	cluster := &Cluster{
		leafSessions: make(map[uuid.UUID]*leafSession),
	}

	config := gossip.DefaultConfig()
	config.GossipInterval = GossipInterval
	cluster.config = config

	if advertiseAddr != "" {

		log.Info().Msgf("cluster: enabling cluster mode on %s", advertiseAddr)

		db := database.GetInstance()
		nodeId, err := db.GetCfgValue("node_id")
		if err != nil || nodeId.Value == "" {
			log.Fatal().Msg("server: node_id not set")
		}

		config.NodeID = nodeId.Value
		config.BindAddr = bindAddr
		config.AdvertiseAddr = advertiseAddr

		if !strings.HasPrefix(config.AdvertiseAddr, "wss://") && !strings.HasPrefix(config.AdvertiseAddr, "https://") {
			config.EncryptionKey = []byte(clusterKey)
			config.Cipher = encryption.NewAESEncryptor()
			config.SocketTransportEnabled = true

			if compress {
				config.Compressor = compression.NewSnappyCompressor()
			}
		} else {
			config.WebsocketProvider = websocket.NewGorillaProvider(5*time.Second, true, clusterKey)
			config.SocketTransportEnabled = false
			config.BearerToken = clusterKey

			url, err := url.Parse(config.AdvertiseAddr)
			if err != nil {
				log.Fatal().Msgf("cluster: failed to parse advertise URL %s: %s", config.AdvertiseAddr, err.Error())
			}

			url.Path = "/cluster"
			config.AdvertiseAddr = url.String()
		}

		config.Logger = common.NewZerologLogger(log.Logger)
		config.MsgCodec = codec.NewVmihailencoMsgpackCodec()

		config.ApplicationVersion = build.Version
		config.ApplicationVersionCheck = func(version string) bool {
			ourParts := strings.Split(build.Version, ".")
			versionParts := strings.Split(version, ".")
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
		cluster.gossipCluster.HandleFunc(AuditLogGossipMsg, cluster.handleAuditLogGossip)

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

		cluster.gossipCluster.LocalMetadata().SetString("location", cfg.Location)
	}

	// Setup routes for leaf nodes
	routes.HandleFunc("GET /cluster/leaf", middleware.ApiAuth(cluster.HandleLeafServer))

	return cluster
}

func (c *Cluster) Start(peers []string, originServer string, originToken string) {
	if c.gossipCluster != nil {
		log.Info().Msg("cluster: starting gossip cluster")
		c.gossipCluster.Start()

		// Process the peers list, any that start with ws://, wss://, http:// or https:// need the path to be /cluster
		for i, peer := range peers {
			if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") || strings.HasPrefix(peer, "http://") || strings.HasPrefix(peer, "https://") {
				url, err := url.Parse(peer)
				if err != nil {
					log.Fatal().Msgf("cluster: failed to parse peer URL %s: %s", peer, err.Error())
				}

				url.Path = "/cluster"
				peers[i] = url.String()
			}
		}

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
	} else if originServer != "" && originToken != "" {
		c.runLeafClient(originServer, originToken)

		// Periodically gossip objects to leaf nodes
		go func() {
			interval := time.NewTicker(c.config.GossipInterval)
			defer interval.Stop()

			for range interval.C {
				c.gossipGroups()
				c.gossipRoles()
				c.gossipTemplates()
				c.gossipTemplateVars()
				c.gossipUsers()
			}
		}()
	}
}

func (c *Cluster) Stop() {
	if c.gossipCluster != nil {
		log.Info().Msg("cluster: stopping gossip cluster")
		c.gossipCluster.Stop()
	}
}

func (c *Cluster) Nodes() []*gossip.Node {
	if c.gossipCluster != nil {
		return c.gossipCluster.Nodes()
	}
	return nil
}

func (c *Cluster) getBatchSize(totalNodes int) int {
	if totalNodes <= 0 {
		return 0
	}

	basePeerCount := math.Ceil(math.Log2(float64(totalNodes))*c.config.StateExchangeMultiplier) + 2
	size := int(math.Max(1, math.Min(basePeerCount, 16.0)))
	if size > totalNodes {
		return totalNodes
	}
	return size
}
