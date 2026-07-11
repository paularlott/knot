package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleCommandFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received command full sync request")

	commands := []*model.Command{}
	if err := packet.Unmarshal(&commands); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal command full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingCommands, err := db.GetCommands()
	if err != nil {
		return nil, err
	}

	go c.mergeCommands(commands)

	return existingCommands, nil
}

func (c *Cluster) handleCommandGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Trace("Received command gossip request")

	commands := []*model.Command{}
	if err := packet.Unmarshal(&commands); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal command gossip request")
		return err
	}

	if err := c.mergeCommands(commands); err != nil {
		c.logger.WithError(err).Error("Failed to merge commands")
		return err
	}

	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipCommand, &commands)
	}

	return nil
}

func (c *Cluster) GossipCommand(command *model.Command) {
	if c.gossipCluster != nil {
		c.logger.Trace("Gossipping command")

		commands := []*model.Command{command}
		c.gossipCluster.Send(CommandGossipMsg, &commands)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Trace("Updating command on leaf nodes")

		commands := []*model.Command{command}
		c.sendToLeafNodes(leafmsg.MessageGossipCommand, &commands)
	}
}

func (c *Cluster) DoCommandFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		commands, err := db.GetCommands()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, CommandFullSyncMsg, &commands, &commands); err != nil {
			return err
		}

		if err := c.mergeCommands(commands); err != nil {
			c.logger.WithError(err).Error("Failed to merge commands")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeCommands(commands []*model.Command) error {
	c.logger.Trace("Merging commands", "number_commands", len(commands))

	db := database.GetInstance()
	localCommands, err := db.GetCommands()
	if err != nil {
		return err
	}

	localCommandsMap := make(map[string]*model.Command)
	for _, command := range localCommands {
		localCommandsMap[command.Id] = command
	}

	for _, command := range commands {
		if localCommand, ok := localCommandsMap[command.Id]; ok {
			if command.UpdatedAt.After(localCommand.UpdatedAt) {
				if err := db.SaveCommand(command, nil); err != nil {
					c.logger.Error("Failed to update command", "error", err, "name", command.Name)
				}

				if command.IsDeleted {
					sse.PublishSlashCommandsDeleted(command.Id)
				} else {
					sse.PublishSlashCommandsChanged(command.Id)
				}
			}
		} else {
			if err := db.SaveCommand(command, nil); err != nil {
				c.logger.Error("Failed to save command", "error", err, "name", command.Name, "is_deleted", command.IsDeleted)
			}

			if !command.IsDeleted {
				sse.PublishSlashCommandsChanged(command.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipCommands() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	db := database.GetInstance()
	commands, err := db.GetCommands()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get commands")
		return
	}

	activeCommands := []*model.Command{}
	for _, command := range commands {
		if command.Active {
			activeCommands = append(activeCommands, command)
		}
	}

	rand.Shuffle(len(commands), func(i, j int) {
		commands[i], commands[j] = commands[j], commands[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(commands))
		if batchSize > 0 {
			c.logger.Trace("Gossipping commands", "batch_size", batchSize, "total", len(commands))
			clusterCommands := commands[:batchSize]
			c.gossipCluster.Send(CommandGossipMsg, &clusterCommands)
		}
	}

	if len(c.leafSessions) > 0 && len(activeCommands) > 0 {
		rand.Shuffle(len(activeCommands), func(i, j int) {
			activeCommands[i], activeCommands[j] = activeCommands[j], activeCommands[i]
		})
		batchSize := c.CalcLeafPayloadSize(len(activeCommands))
		if batchSize > 0 {
			c.logger.Trace("Commands to leaf nodes", "batch_size", batchSize, "total", len(activeCommands))
			leafCommands := activeCommands[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipCommand, &leafCommands)
		}
	}
}
