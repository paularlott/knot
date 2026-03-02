package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/paularlott/knot/internal/util"
)

func (db *RedisDbDriver) SaveSkill(skill *model.Skill, updateFields []string) error {
	existingSkill, _ := db.GetSkill(skill.Id)

	if existingSkill != nil {
		if (existingSkill.Name != skill.Name || existingSkill.UserId != skill.UserId) && (len(updateFields) == 0 || util.InArray(updateFields, "Name") || util.InArray(updateFields, "UserId")) {
			db.connection.Del(context.Background(), fmt.Sprintf("%sSkillsByName:%s:%s", db.prefix, existingSkill.UserId, existingSkill.Name))
		}

		if len(updateFields) > 0 {
			util.CopyFields(skill, existingSkill, updateFields)
			skill = existingSkill
		}
	}

	data, err := json.Marshal(skill)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSkills:%s", db.prefix, skill.Id), data, 0).Err()
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sSkillsByName:%s:%s", db.prefix, skill.UserId, skill.Name), skill.Id, 0).Err()
}

func (db *RedisDbDriver) DeleteSkill(skill *model.Skill) error {
	db.connection.Del(context.Background(), fmt.Sprintf("%sSkillsByName:%s:%s", db.prefix, skill.UserId, skill.Name))
	return db.connection.Del(context.Background(), fmt.Sprintf("%sSkills:%s", db.prefix, skill.Id)).Err()
}

func (db *RedisDbDriver) GetSkill(id string) (*model.Skill, error) {
	var skill = &model.Skill{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSkills:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &skill)
	if err != nil {
		return nil, err
	}

	return skill, nil
}

func (db *RedisDbDriver) GetSkills() ([]*model.Skill, error) {
	var skills []*model.Skill

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSkills:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		skill, err := db.GetSkill(iter.Val()[len(fmt.Sprintf("%sSkills:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		skills = append(skills, skill)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	sort.Slice(skills, func(i, j int) bool {
		return skills[i].Name < skills[j].Name
	})

	return skills, nil
}

func (db *RedisDbDriver) GetSkillsByName(name string) ([]*model.Skill, error) {
	skills, err := db.GetSkills()
	if err != nil {
		return nil, err
	}

	var result []*model.Skill
	for _, skill := range skills {
		if skill.Name == name && skill.UserId == "" {
			result = append(result, skill)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}

func (db *RedisDbDriver) GetSkillsByNameAndUser(name string, userId string) ([]*model.Skill, error) {
	skills, err := db.GetSkills()
	if err != nil {
		return nil, err
	}

	var result []*model.Skill
	for _, skill := range skills {
		if skill.Name == name && skill.UserId == userId {
			result = append(result, skill)
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("skill not found")
	}

	sort.Slice(result, func(i, j int) bool {
		zonesI := len(result[i].Zones)
		zonesJ := len(result[j].Zones)
		if zonesI != zonesJ {
			return zonesI > zonesJ
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})

	return result, nil
}
