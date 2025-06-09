package cluster

import (
	"errors"
	"math"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/paularlott/gossip"
	"github.com/paularlott/gossip/codec"
	"github.com/paularlott/gossip/compression"
	"github.com/paularlott/gossip/encryption"
	"github.com/paularlott/gossip/examples/common"
	"github.com/paularlott/gossip/leader"
	"github.com/paularlott/gossip/websocket"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	cfg "github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

const (
	GossipInterval = 15 * time.Second
)

type Cluster struct {
	gossipCluster    *gossip.Cluster
	config           *gossip.Config
	leafSessionMux   sync.RWMutex
	leafSessions     map[uuid.UUID]*leafSession
	agentEndpoints   []string
	sessionGossip    bool
	election         *leader.LeaderElection
	resourceLocksMux sync.RWMutex
	resourceLocks    map[string]*ResourceLock
}

func NewCluster(
	clusterKey string,
	advertiseAddr string,
	bindAddr string,
	routes *http.ServeMux,
	compress bool,
	allowLeaf bool,
	agentEndpoints []string,
) *Cluster {
	cluster := &Cluster{
		leafSessions:   make(map[uuid.UUID]*leafSession),
		agentEndpoints: agentEndpoints,
		sessionGossip:  !database.IsSessionDriverShared(),
		resourceLocks:  make(map[string]*ResourceLock),
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
		cluster.gossipCluster.HandleFuncWithReply(TokenFullSyncMsg, cluster.handleTokenFullSync)
		cluster.gossipCluster.HandleFunc(TokenGossipMsg, cluster.handleTokenGossip)
		cluster.gossipCluster.HandleFuncWithReply(VolumeFullSyncMsg, cluster.handleVolumeFullSync)
		cluster.gossipCluster.HandleFunc(VolumeGossipMsg, cluster.handleVolumeGossip)
		cluster.gossipCluster.HandleFunc(AuditLogGossipMsg, cluster.handleAuditLogGossip)
		cluster.gossipCluster.HandleFuncWithReply(ResourceLockFullSyncMsg, cluster.handleResourceLockFullSync)
		cluster.gossipCluster.HandleFunc(ResourceLockGossipMsg, cluster.handleResourceLockGossip)

		if cluster.sessionGossip {
			cluster.gossipCluster.HandleFuncWithReply(SessionFullSyncMsg, cluster.handleSessionFullSync)
			cluster.gossipCluster.HandleFunc(SessionGossipMsg, cluster.handleSessionGossip)
		}

		cluster.gossipCluster.HandleFuncWithReply(ResourceLockMsg, cluster.handleResourceLock)
		cluster.gossipCluster.HandleFunc(ResourceUnlockMsg, cluster.handleResourceUnlock)

		// Capture server state changes and maintain a list of nodes in our location
		// We only dynamically track nodes if the endpoint list hans't been set.
		if len(agentEndpoints) == 0 {
			cluster.gossipCluster.HandleNodeStateChangeFunc(func(node *gossip.Node, prevState gossip.NodeState) {
				nodes := cluster.gossipCluster.AliveNodes()
				endPoints := []string{}
				for _, n := range nodes {
					if n.Metadata.GetString("location") == cfg.Location {
						endPoints = append(endPoints, n.Metadata.GetString("agent_endpoint"))
					}
				}
				cluster.agentEndpoints = endPoints
			})
		}

		// Periodically gossip the status of the objects
		cluster.gossipCluster.HandleGossipFunc(func() {
			cluster.gossipGroups()
			cluster.gossipRoles()
			cluster.gossipSpaces()
			cluster.gossipTemplates()
			cluster.gossipTemplateVars()
			cluster.gossipUsers()
			cluster.gossipTokens()
			cluster.gossipVolumes()
			cluster.gossipResourceLocks()
			if cluster.sessionGossip {
				cluster.gossipSessions()
			}
		})

		metadata := cluster.gossipCluster.LocalMetadata()
		metadata.SetString("location", cfg.Location)
		metadata.SetString("agent_endpoint", viper.GetString("server.agent_endpoint"))

		// Set up leader elections within the locality
		electionCfg := leader.DefaultConfig()
		electionCfg.MetadataFilterKey = "location"
		cluster.election = leader.NewLeaderElection(cluster.gossipCluster, electionCfg)
	}

	if allowLeaf {
		log.Info().Msg("cluster: enabling support for leaf nodes")

		// Setup routes for leaf nodes
		routes.HandleFunc("GET /cluster/leaf", middleware.ApiAuth(cluster.HandleLeafServer))
	}

	// Go routine to periodically clean up the resource locks
	go func() {
		interval := time.NewTicker(ResourceLockGCInterval)
		defer interval.Stop()

		for range interval.C {
			log.Debug().Msg("cluster: cleaning up resource locks")
			cluster.resourceLocksMux.Lock()
			for id, lock := range cluster.resourceLocks {
				if lock.ExpiresAfter.Before(time.Now().UTC()) {
					log.Debug().Msgf("cluster: removing expired resource lock %s", id)
					delete(cluster.resourceLocks, id)
				}
			}
			cluster.resourceLocksMux.Unlock()
		}
	}()

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

					if err := c.DoTokenFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync tokens with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoVolumeFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync volumes with node %s: %s", node.ID, err.Error())
					}

					if err := c.DoResourceLockFullSync(node); err != nil {
						log.Error().Msgf("cluster: failed to sync resource locks with node %s: %s", node.ID, err.Error())
					}

					if c.sessionGossip {
						if err := c.DoSessionFullSync(node); err != nil {
							log.Error().Msgf("cluster: failed to sync sessions with node %s: %s", node.ID, err.Error())
						}
					}
				}

				log.Info().Msg("cluster: full state sync complete")
			}()
		}

		// Start the leader election process
		c.election.Start()
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
		c.election.Stop()
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

func (c *Cluster) GetAgentEndpoints() []string {
	return c.agentEndpoints
}

func (c *Cluster) LockResource(resourceId string) string {
	// If in cluster mode and not the leader then we have to ask the leader to lock the resource
	if c.election != nil && !c.election.IsLeader() {
		log.Debug().Msg("cluster: Asking leader to lock resource")

		leaderNode := c.election.GetLeader()
		if leaderNode != nil {
			request := &ResourceLockRequestMsg{
				ResourceId: resourceId,
			}
			response := &ResourceLockResponseMsg{}
			if err := c.gossipCluster.SendToWithResponse(leaderNode, ResourceLockMsg, request, ResourceLockMsg, response); err != nil {
				log.Error().Msgf("cluster: Failed to request resource lock from leader %s: %s", leaderNode.ID, err.Error())
				return ""
			}

			return response.UnlockToken
		}
	}

	return c.lockResourceLocally(resourceId)
}

func (c *Cluster) lockResourceLocally(resourceId string) string {
	c.resourceLocksMux.Lock()
	defer c.resourceLocksMux.Unlock()

	if lock, exists := c.resourceLocks[resourceId]; exists {
		if lock.ExpiresAfter.After(time.Now().UTC()) {
			return ""
		}
	}

	lock := &ResourceLock{
		Id:           resourceId,
		UnlockToken:  crypt.CreateKey(),
		UpdatedAt:    time.Now().UTC(),
		ExpiresAfter: time.Now().UTC().Add(ResourceLockTTL),
	}
	c.resourceLocks[resourceId] = lock
	c.GossipResourceLock(lock)

	return lock.UnlockToken
}

func (c *Cluster) UnlockResource(resourceId, unlockToken string) {
	// If in cluster mode and not the leader then we have to ask the leader to unlock the resource
	if c.election != nil && !c.election.IsLeader() {
		log.Debug().Msg("cluster: Asking leader to unlock resource")

		leaderNode := c.election.GetLeader()
		if leaderNode != nil {
			request := &ResourceUnlockRequestMsg{
				ResourceId:  resourceId,
				UnlockToken: unlockToken,
			}
			if err := c.gossipCluster.SendTo(leaderNode, ResourceUnlockMsg, request); err != nil {
				log.Error().Msgf("cluster: Failed to request resource unlock from leader %s: %s", leaderNode.ID, err.Error())
			}

			return
		}
	}

	c.unlockResourceLocally(resourceId, unlockToken)
}

func (c *Cluster) unlockResourceLocally(resourceId, unlockToken string) {
	c.resourceLocksMux.Lock()
	defer c.resourceLocksMux.Unlock()

	if lock, exists := c.resourceLocks[resourceId]; exists {
		if lock.UnlockToken == unlockToken {
			delete(c.resourceLocks, resourceId)
			lock.UpdatedAt = time.Now().UTC()
			lock.ExpiresAfter = time.Now().UTC().Add(-ResourceLockTTL)
			c.GossipResourceLock(lock)
		}
	}
}

func (c *Cluster) handleResourceLock(sender *gossip.Node, packet *gossip.Packet) (gossip.MessageType, interface{}, error) {
	// If the sender doesn't match our location then ignore the request
	if sender.Metadata.GetString("location") != config.Location {
		log.Debug().Msg("cluster: Ignoring resource lock request from a different location")
		return gossip.NilMsg, nil, errors.New("resource lock request from different location")
	}

	request := ResourceLockRequestMsg{}
	if err := packet.Unmarshal(&request); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal resource lock request")
		return gossip.NilMsg, nil, err
	}

	response := &ResourceLockResponseMsg{
		UnlockToken: c.lockResourceLocally(request.ResourceId),
	}

	// Return the full dataset directly as response
	return ResourceLockMsg, response, nil
}

func (c *Cluster) handleResourceUnlock(sender *gossip.Node, packet *gossip.Packet) error {
	// If the sender doesn't match our location then ignore the request
	if sender.Metadata.GetString("location") != config.Location {
		log.Debug().Msg("cluster: Ignoring resource unlock request from a different location")
		return errors.New("resource unlock request from different location")
	}

	request := ResourceUnlockRequestMsg{}
	if err := packet.Unmarshal(&request); err != nil {
		log.Error().Err(err).Msg("cluster: Failed to unmarshal resource lock request")
		return err
	}

	c.unlockResourceLocally(request.ResourceId, request.UnlockToken)

	return nil
}
