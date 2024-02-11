package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveTemplateVar(templateVar *model.TemplateVar) error {

  // Load the existing template var
  existingTemplateVar, _ := db.GetTemplate(templateVar.Id)
  if existingTemplateVar == nil {
    templateVar.CreatedAt = time.Now().UTC()
  }

  templateVar.Value = templateVar.GetValueEncrypted()
  templateVar.UpdatedAt = time.Now().UTC()
  data, err := json.Marshal(templateVar)
  if err != nil {
    return err
  }

  return db.connection.Set(context.Background(), fmt.Sprintf("TemplateVars:%s", templateVar.Id), data, 0).Err()
}

func (db *RedisDbDriver) DeleteTemplateVar(templateVar *model.TemplateVar) error {
  return db.connection.Del(context.Background(), fmt.Sprintf("TemplateVars:%s", templateVar.Id)).Err()
}

func (db *RedisDbDriver) GetTemplateVar(id string) (*model.TemplateVar, error) {
  var templateVar = &model.TemplateVar{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("TemplateVars:%s", id)).Result()
  if err != nil {
    return nil, convertRedisError(err)
  }

  err = json.Unmarshal([]byte(v), &templateVar)
  if err != nil {
    return nil, err
  }

  templateVar.DecryptSetValue(templateVar.Value)

  return templateVar, nil
}

func (db *RedisDbDriver) GetTemplateVars() ([]*model.TemplateVar, error) {
  var templateVars []*model.TemplateVar

  iter := db.connection.Scan(context.Background(), 0, "TemplateVars:*", 0).Iterator()
  for iter.Next(context.Background()) {
    templateVar, err := db.GetTemplateVar(iter.Val()[13:])
    if err != nil {
      return nil, err
    }

    templateVar.DecryptSetValue(templateVar.Value)
    templateVars = append(templateVars, templateVar)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  // Sort the template vars by name
  sort.Slice(templateVars, func(i, j int) bool {
    return templateVars[i].Name < templateVars[j].Name
  })

  return templateVars, nil
}
