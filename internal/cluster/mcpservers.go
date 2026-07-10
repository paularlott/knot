package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleMCPServerFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received MCP server full sync request")

	servers := []*model.MCPServer{}
	if err := packet.Unmarshal(&servers); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal MCP server full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingServers, err := db.GetMCPServers()
	if err != nil {
		return nil, err
	}

	go c.mergeMCPServers(servers)

	return existingServers, nil
}

func (c *Cluster) handleMCPServerGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received MCP server gossip request")

	servers := []*model.MCPServer{}
	if err := packet.Unmarshal(&servers); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal MCP server gossip request")
		return err
	}

	if err := c.mergeMCPServers(servers); err != nil {
		c.logger.WithError(err).Error("Failed to merge MCP servers")
		return err
	}

	// NOTE: MCP servers are NOT forwarded to leaf nodes per design.
	// Leaf servers manage their own MCP servers locally.

	return nil
}

func (c *Cluster) GossipMCPServer(server *model.MCPServer) {
	if c.gossipCluster != nil {
		c.logger.Trace("Gossipping MCP server")

		servers := []*model.MCPServer{server}
		c.gossipCluster.Send(MCPServerGossipMsg, &servers)
	}

	// NOTE: MCP servers are NOT sent to leaf nodes per design.
}

func (c *Cluster) DoMCPServerFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		servers, err := db.GetMCPServers()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, MCPServerFullSyncMsg, &servers, &servers); err != nil {
			return err
		}

		if err := c.mergeMCPServers(servers); err != nil {
			c.logger.WithError(err).Error("Failed to merge MCP servers")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeMCPServers(servers []*model.MCPServer) error {
	c.logger.Trace("Merging MCP servers", "number_servers", len(servers))

	db := database.GetInstance()
	localServers, err := db.GetMCPServers()
	if err != nil {
		return err
	}

	localServersMap := make(map[string]*model.MCPServer)
	for _, s := range localServers {
		localServersMap[s.Id] = s
	}

	for _, server := range servers {
		if localServer, ok := localServersMap[server.Id]; ok {
			if server.UpdatedAt.After(localServer.UpdatedAt) {
				if err := db.SaveMCPServer(server, nil); err != nil {
					c.logger.Error("Failed to update MCP server", "error", err, "namespace", server.Namespace)
				}

				if server.IsDeleted {
					sse.PublishMCPServersDeleted(server.Id)
				} else {
					sse.PublishMCPServersChanged(server.Id)
				}
			}
		} else {
			if err := db.SaveMCPServer(server, nil); err != nil {
				c.logger.Error("Failed to save MCP server", "error", err, "namespace", server.Namespace, "is_deleted", server.IsDeleted)
			}

			if !server.IsDeleted {
				sse.PublishMCPServersChanged(server.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipMCPServers() {
	if c.gossipCluster == nil {
		return
	}

	// NOTE: Leaf sessions are intentionally skipped — MCP servers are NOT
	// replicated to leaf nodes. Leaf nodes manage their own MCP servers.

	db := database.GetInstance()
	servers, err := db.GetMCPServers()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get MCP servers")
		return
	}

	rand.Shuffle(len(servers), func(i, j int) {
		servers[i], servers[j] = servers[j], servers[i]
	})

	batchSize := c.gossipCluster.CalcPayloadSize(len(servers))
	if batchSize == 0 {
		return
	}

	c.logger.Trace("Gossipping MCP servers", "batch_size", batchSize, "total", len(servers))

	clusterServers := servers[:batchSize]
	c.gossipCluster.Send(MCPServerGossipMsg, &clusterServers)
}
