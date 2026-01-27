package cluster

import (
	"math/rand"

	"github.com/paularlott/gossip"
	"github.com/paularlott/knot/internal/cluster/leafmsg"
	"github.com/paularlott/knot/internal/database"
	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/sse"
)

func (c *Cluster) handleSkillFullSync(sender *gossip.Node, packet *gossip.Packet) (interface{}, error) {
	c.logger.Debug("Received skill full sync request")

	skills := []*model.Skill{}
	if err := packet.Unmarshal(&skills); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal skill full sync request")
		return nil, err
	}

	db := database.GetInstance()
	existingSkills, err := db.GetSkills()
	if err != nil {
		return nil, err
	}

	go c.mergeSkills(skills)

	return existingSkills, nil
}

func (c *Cluster) handleSkillGossip(sender *gossip.Node, packet *gossip.Packet) error {
	c.logger.Debug("Received skill gossip request")

	skills := []*model.Skill{}
	if err := packet.Unmarshal(&skills); err != nil {
		c.logger.WithError(err).Error("Failed to unmarshal skill gossip request")
		return err
	}

	if err := c.mergeSkills(skills); err != nil {
		c.logger.WithError(err).Error("Failed to merge skills")
		return err
	}

	if len(c.leafSessions) > 0 {
		c.sendToLeafNodes(leafmsg.MessageGossipSkill, &skills)
	}

	return nil
}

func (c *Cluster) GossipSkill(skill *model.Skill) {
	if c.gossipCluster != nil {
		c.logger.Debug("Gossipping skill")

		skills := []*model.Skill{skill}
		c.gossipCluster.Send(SkillGossipMsg, &skills)
	}

	if len(c.leafSessions) > 0 {
		c.logger.Debug("Updating skill on leaf nodes")

		skills := []*model.Skill{skill}
		c.sendToLeafNodes(leafmsg.MessageGossipSkill, skills)
	}
}

func (c *Cluster) DoSkillFullSync(node *gossip.Node) error {
	if c.gossipCluster != nil {
		db := database.GetInstance()
		skills, err := db.GetSkills()
		if err != nil {
			return err
		}

		if err := c.gossipCluster.SendToWithResponse(node, SkillFullSyncMsg, &skills, &skills); err != nil {
			return err
		}

		if err := c.mergeSkills(skills); err != nil {
			c.logger.WithError(err).Error("Failed to merge skills")
			return err
		}
	}

	return nil
}

func (c *Cluster) mergeSkills(skills []*model.Skill) error {
	c.logger.Debug("Merging skills", "number_skills", len(skills))

	db := database.GetInstance()
	localSkills, err := db.GetSkills()
	if err != nil {
		return err
	}

	localSkillsMap := make(map[string]*model.Skill)
	for _, skill := range localSkills {
		localSkillsMap[skill.Id] = skill
	}

	for _, skill := range skills {
		if localSkill, ok := localSkillsMap[skill.Id]; ok {
			if skill.UpdatedAt.After(localSkill.UpdatedAt) {
				if err := db.SaveSkill(skill, nil); err != nil {
					c.logger.Error("Failed to update skill", "error", err, "name", skill.Name)
				}

				if skill.IsDeleted {
					sse.PublishSkillsDeleted(skill.Id)
				} else {
					sse.PublishSkillsChanged(skill.Id)
				}
			}
		} else {
			if err := db.SaveSkill(skill, nil); err != nil {
				c.logger.Error("Failed to save skill", "error", err, "name", skill.Name, "is_deleted", skill.IsDeleted)
			}

			if !skill.IsDeleted {
				sse.PublishSkillsChanged(skill.Id)
			}
		}
	}

	return nil
}

func (c *Cluster) gossipSkills() {
	if c.gossipCluster == nil && len(c.leafSessions) == 0 {
		return
	}

	db := database.GetInstance()
	skills, err := db.GetSkills()
	if err != nil {
		c.logger.WithError(err).Error("Failed to get skills")
		return
	}

	rand.Shuffle(len(skills), func(i, j int) {
		skills[i], skills[j] = skills[j], skills[i]
	})

	if c.gossipCluster != nil {
		batchSize := c.gossipCluster.CalcPayloadSize(len(skills))
		if batchSize > 0 {
			c.logger.Debug("Gossipping skills", "batch_size", batchSize, "total", len(skills))
			clusterSkills := skills[:batchSize]
			c.gossipCluster.Send(SkillGossipMsg, &clusterSkills)
		}
	}

	if len(c.leafSessions) > 0 {
		batchSize := c.CalcLeafPayloadSize(len(skills))
		if batchSize > 0 {
			c.logger.Debug("Skills to leaf nodes", "batch_size", batchSize, "total", len(skills))
			leafSkills := skills[:batchSize]
			c.sendToLeafNodes(leafmsg.MessageGossipSkill, &leafSkills)
		}
	}
}
