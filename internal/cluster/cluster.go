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
	"github.com/paularlott/gossip/hlc"
	"github.com/paularlott/gossip/leader"
	"github.com/paularlott/knot/build"
	"github.com/paularlott/knot/internal/config"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/dns"
	"github.com/paularlott/knot/internal/middleware"
	"github.com/paularlott/knot/internal/util/crypt"

	"github.com/google/uuid"
	"github.com/paularlott/knot/internal/log"
)

type Cluster struct {
	gossipCluster    *gossip.Cluster
	config           *gossip.Config
	leafSessionMux   sync.RWMutex
	leafSessions     map[uuid.UUID]*leafSession
	agentEndpoints   []string
	tunnelServers    []string
	sessionGossip    bool
	election         *leader.LeaderElection
	electionRunning  bool
	electionMux      sync.Mutex
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
) *Cluster {
	cluster := &Cluster{
		leafSessions:   make(map[uuid.UUID]*leafSession),
		agentEndpoints: []string{},
		tunnelServers:  []string{},
		sessionGossip:  !database.IsSessionDriverShared(),
		resourceLocks:  make(map[string]*ResourceLock),
	}

	gossipConfig := gossip.DefaultConfig()
	gossipConfig.Resolver = dns.GetDefaultResolver()
	cluster.config = gossipConfig

	if advertiseAddr != "" {
		log.Info("cluster: enabling cluster mode on", "advertiseAddr", advertiseAddr)

		db := database.GetInstance()
		nodeId, err := db.GetCfgValue("node_id")
		if err != nil || nodeId.Value == "" {
			log.Fatal("server: node_id not set")
		}

		gossipConfig.NodeID = nodeId.Value
		gossipConfig.AdvertiseAddr = advertiseAddr

		var httpTransport *gossip.HTTPTransport
		if !strings.HasPrefix(gossipConfig.AdvertiseAddr, "https://") && !strings.HasPrefix(gossipConfig.AdvertiseAddr, "http://") {
			gossipConfig.BindAddr = bindAddr
			gossipConfig.EncryptionKey = []byte(clusterKey)
			gossipConfig.Cipher = encryption.NewAESEncryptor()
			gossipConfig.Transport = gossip.NewSocketTransport(gossipConfig)

			if compress {
				gossipConfig.Compressor = compression.NewSnappyCompressor()
			}
		} else {
			gossipConfig.BearerToken = clusterKey
			gossipConfig.BindAddr = "/cluster"
			httpTransport = gossip.NewHTTPTransport(gossipConfig)
			gossipConfig.Transport = httpTransport

			url, err := url.Parse(gossipConfig.AdvertiseAddr)
			if err != nil {
				log.WithError(err).Fatal("cluster: failed to parse advertise URL :")
			}

			url.Path = "/cluster"
			gossipConfig.AdvertiseAddr = url.String()
		}

		gossipConfig.Logger = log.GetLogger().WithGroup("gossip")
		gossipConfig.MsgCodec = codec.NewVmihailencoMsgpackCodec()

		gossipConfig.ApplicationVersion = build.Version
		gossipConfig.ApplicationVersionCheck = func(version string) bool {
			ourParts := strings.Split(build.Version, ".")
			versionParts := strings.Split(version, ".")
			if len(ourParts) < 2 || len(versionParts) < 2 || ourParts[0] != versionParts[0] || ourParts[1] != versionParts[1] {
				return false
			}
			return true
		}

		cluster.gossipCluster, err = gossip.NewCluster(gossipConfig)
		if err != nil {
			log.WithError(err).Fatal("cluster: failed to create gossip cluster:")
		}

		// If using websockets then add the handler
		if httpTransport != nil {
			routes.HandleFunc("POST /cluster", httpTransport.HandleGossipRequest)
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

		// Capture server state changes and maintain a list of nodes in our zone
		cluster.gossipCluster.HandleNodeStateChangeFunc(func(node *gossip.Node, prevState gossip.NodeState) {
			cluster.trackClusterEndpoints()
			cluster.manageElection()
		})
		cluster.gossipCluster.HandleNodeMetadataChangeFunc(func(node *gossip.Node) {
			cluster.trackClusterEndpoints()
			cluster.manageElection()
		})

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

		cfg := config.GetServerConfig()
		metadata := cluster.gossipCluster.LocalMetadata()
		metadata.SetString("zone", cfg.Zone)
		metadata.SetString("agent_endpoint", cfg.AgentEndpoint)
		metadata.SetString("tunnel_server", cfg.TunnelServer)

		// Set up leader elections within the locality
		electionCfg := leader.DefaultConfig()
		electionCfg.MetadataCriteria = map[string]string{
			"zone": cfg.Zone,
		}
		cluster.election = leader.NewLeaderElection(cluster.gossipCluster, electionCfg)
	}

	if allowLeaf {
		log.Info("cluster: enabling support for leaf nodes")

		// Setup routes for leaf nodes
		routes.HandleFunc("GET /cluster/leaf", middleware.ApiAuth(cluster.HandleLeafServer))
	}

	// Go routine to periodically clean up the resource locks
	go func() {
		interval := time.NewTicker(ResourceLockGCInterval)
		defer interval.Stop()

		for range interval.C {
			log.Debug("cluster: cleaning up resource locks")
			cluster.resourceLocksMux.Lock()
			for id, lock := range cluster.resourceLocks {
				if lock.ExpiresAfter.Before(time.Now().UTC()) {
					log.Debug("cluster: removing expired resource lock", "id", id)
					delete(cluster.resourceLocks, id)
				}
			}
			cluster.resourceLocksMux.Unlock()
		}
	}()

	return cluster
}

func (c *Cluster) trackClusterEndpoints() {
	cfg := config.GetServerConfig()
	nodes := c.gossipCluster.AliveNodes()
	endPoints := []string{}
	tunnelServers := []string{}
	for _, n := range nodes {
		if n.Metadata.GetString("zone") == cfg.Zone {
			if n.Metadata.GetString("agent_endpoint") != "" {
				endPoints = append(endPoints, n.Metadata.GetString("agent_endpoint"))
			}
			if n.Metadata.GetString("tunnel_server") != "" {
				tunnelServers = append(tunnelServers, n.Metadata.GetString("tunnel_server"))
			}
		}
	}
	c.agentEndpoints = endPoints
	c.tunnelServers = tunnelServers
}

func (c *Cluster) manageElection() {
	if c.election == nil || c.electionRunning {
		return
	}

	c.electionMux.Lock()
	defer c.electionMux.Unlock()

	// Count nodes in our zone
	cfg := config.GetServerConfig()
	nodesInZone := 0
	for _, node := range c.gossipCluster.AliveNodes() {
		if node.Metadata.GetString("zone") == cfg.Zone {
			nodesInZone++
		}
	}

	if nodesInZone >= 2 {
		log.Info("cluster: starting leader election with nodes in zone", "nodesInZone", nodesInZone)
		c.election.Start()
		c.electionRunning = true
	}
}

func (c *Cluster) Start(peers []string, originServer string, originToken string) {
	if c.gossipCluster != nil {
		log.Info("cluster: starting gossip cluster")
		c.gossipCluster.Start()

		// Process the peers list, any that start with ws://, wss://, http:// or https:// need the path to be /cluster
		for i, peer := range peers {
			if strings.HasPrefix(peer, "ws://") || strings.HasPrefix(peer, "wss://") || strings.HasPrefix(peer, "http://") || strings.HasPrefix(peer, "https://") {
				url, err := url.Parse(peer)
				if err != nil {
					log.WithError(err).Fatal("cluster: failed to parse peer URL :")
				}

				url.Path = "/cluster"
				peers[i] = url.String()
			}
		}

		// Join the initial peers
		if err := c.gossipCluster.Join(peers); err != nil {
			log.WithError(err).Fatal("cluster: failed to join cluster:")
		}

		// If the cluster is bigger than us then trigger a full state sync
		if c.gossipCluster.NumAliveNodes() > 1 {
			go func() {
				log.Info("cluster: starting full state sync")

				nodes := c.gossipCluster.GetCandidates()

				// Try each node in order until we get a successful response
				for _, node := range nodes {
					if err := c.DoGroupFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync groups with node :")
					}

					if err := c.DoRoleFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync roles with node :")
					}

					if err := c.DoSpaceFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync spaces with node :")
					}

					if err := c.DoTemplateFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync templates with node :")
					}

					if err := c.DoTemplateVarFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync template vars with node :")
					}

					if err := c.DoUserFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync users with node :")
					}

					if err := c.DoTokenFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync tokens with node :")
					}

					if err := c.DoVolumeFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync volumes with node :")
					}

					if err := c.DoResourceLockFullSync(node); err != nil {
						log.WithError(err).Error("cluster: failed to sync resource locks with node :")
					}

					if c.sessionGossip {
						if err := c.DoSessionFullSync(node); err != nil {
							log.WithError(err).Error("cluster: failed to sync sessions with node :")
						}
					}
				}

				log.Info("cluster: full state sync complete")
			}()
		}

		// Start the leader election process
		c.election.Start()
	} else if originServer != "" && originToken != "" {
		c.runLeafClient(originServer, originToken)
	}
}

func (c *Cluster) Stop() {
	if c.gossipCluster != nil {
		log.Info("cluster: stopping gossip cluster")

		c.electionMux.Lock()
		if c.electionRunning {
			c.election.Stop()
			c.electionRunning = false
		}
		c.electionMux.Unlock()

		c.gossipCluster.Stop()
	}
}

func (c *Cluster) Nodes() []*gossip.Node {
	if c.gossipCluster != nil {
		return c.gossipCluster.Nodes()
	}
	return nil
}

// This is a duplicate of the gossip payload size calculation, however when gossip isn't running we still need this for sending to leaf nodes
func (c *Cluster) CalcLeafPayloadSize(totalNodes int) int {
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

func (c *Cluster) GetTunnelServers() []string {
	return c.tunnelServers
}

func (c *Cluster) LockResource(resourceId string) string {
	// If in cluster mode and not the leader then we have to ask the leader to lock the resource
	if c.election != nil && c.electionRunning && !c.election.IsLeader() {
		log.Debug("cluster: Asking leader to lock resource")

		leaderNode := c.election.GetLeader()
		if leaderNode != nil {
			request := &ResourceLockRequestMsg{
				ResourceId: resourceId,
			}
			response := &ResourceLockResponseMsg{}
			if err := c.gossipCluster.SendToWithResponse(leaderNode, ResourceLockMsg, request, response); err != nil {
				log.WithError(err).Error("cluster: Failed to request resource lock from leader :")
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
		if lock.ExpiresAfter.After(time.Now().UTC()) && !lock.IsDeleted {
			return ""
		}
	}

	lock := &ResourceLock{
		Id:           resourceId,
		UnlockToken:  crypt.CreateKey(),
		UpdatedAt:    hlc.Now(),
		ExpiresAfter: time.Now().UTC().Add(ResourceLockTTL),
	}
	c.resourceLocks[resourceId] = lock
	c.GossipResourceLock(lock)

	return lock.UnlockToken
}

func (c *Cluster) UnlockResource(resourceId, unlockToken string) {
	// If in cluster mode and not the leader then we have to ask the leader to unlock the resource
	if c.election != nil && c.electionRunning && !c.election.IsLeader() {
		log.Debug("cluster: Asking leader to unlock resource")

		leaderNode := c.election.GetLeader()
		if leaderNode != nil {
			request := &ResourceUnlockRequestMsg{
				ResourceId:  resourceId,
				UnlockToken: unlockToken,
			}
			if err := c.gossipCluster.SendTo(leaderNode, ResourceUnlockMsg, request); err != nil {
				log.WithError(err).Error("cluster: Failed to request resource unlock from leader :")
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
			lock.IsDeleted = true
			lock.ExpiresAfter = time.Now().UTC().Add(ResourceLockTTL)
			lock.UpdatedAt = hlc.Now()
			c.GossipResourceLock(lock)
		}
	}
}

func (c *Cluster) handleResourceLock(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		log.Debug("cluster: Ignoring resource lock request from a different zone")
		return nil, errors.New("resource lock request from different zone")
	}

	request := ResourceLockRequestMsg{}
	if err := packet.Unmarshal(&request); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal resource lock request")
		return nil, err
	}

	response := &ResourceLockResponseMsg{
		UnlockToken: c.lockResourceLocally(request.ResourceId),
	}

	// Return the full dataset directly as response
	return response, nil
}

func (c *Cluster) handleResourceUnlock(sender *gossip.Node, packet *gossip.Packet) error {
	// If the sender doesn't match our zone then ignore the request
	cfg := config.GetServerConfig()
	if sender.Metadata.GetString("zone") != cfg.Zone {
		log.Debug("cluster: Ignoring resource unlock request from a different zone")
		return errors.New("resource unlock request from different zone")
	}

	request := ResourceUnlockRequestMsg{}
	if err := packet.Unmarshal(&request); err != nil {
		log.WithError(err).Error("cluster: Failed to unmarshal resource lock request")
		return err
	}

	c.unlockResourceLocally(request.ResourceId, request.UnlockToken)

	return nil
}
