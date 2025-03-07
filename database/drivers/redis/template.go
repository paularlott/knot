package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/database/model"
	"github.com/paularlott/knot/util"
)

func (db *RedisDbDriver) SaveTemplate(template *model.Template, updateFields []string) error {
	// Load the existing template
	existingTemplate, _ := db.GetTemplate(template.Id)
	if existingTemplate == nil {
		template.CreatedAt = time.Now().UTC()
	}

	// Apply changes from new to existing if doing partial update
	if existingTemplate != nil && len(updateFields) > 0 {
		util.CopyFields(template, existingTemplate, updateFields)
		template = existingTemplate
	}

	template.UpdatedUserId = template.CreatedUserId
	template.UpdatedAt = time.Now().UTC()
	data, err := json.Marshal(template)
	if err != nil {
		return err
	}

	return db.connection.Set(context.Background(), fmt.Sprintf("%sTemplates:%s", db.prefix, template.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteTemplate(template *model.Template) error {
	// Test if the space in in use
	spaces, err := db.GetSpacesByTemplateId(template.Id)
	if err != nil {
		return err
	}

	if len(spaces) > 0 {
		return fmt.Errorf("template in use")
	}

	return db.connection.Del(context.Background(), fmt.Sprintf("%sTemplates:%s", db.prefix, template.Id)).Err()
}

func (db *RedisDbDriver) GetTemplate(id string) (*model.Template, error) {
	var template = &model.Template{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sTemplates:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &template)
	if err != nil {
		return nil, err
	}

	return template, nil
}

func (db *RedisDbDriver) GetTemplates() ([]*model.Template, error) {
	var templates []*model.Template

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sTemplates:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		template, err := db.GetTemplate(iter.Val()[len(fmt.Sprintf("%sTemplates:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		templates = append(templates, template)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Sort the templates by name
	sort.Slice(templates, func(i, j int) bool {
		return templates[i].Name < templates[j].Name
	})

	return templates, nil
}
