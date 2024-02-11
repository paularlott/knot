package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveToken(token *model.Token) error {
  // Calculate the expiration time as now + 1 week
  token.ExpiresAfter = time.Now().UTC().Add(time.Hour * 168)

  data, err := json.Marshal(token)
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("Tokens:%s", token.Id), data, time.Hour * 168).Err()
  if err != nil {
    return err
  }

  err = db.connection.Set(context.Background(), fmt.Sprintf("TokensByUserId:%s:%s", token.UserId, token.Id), token.Id, time.Hour * 168).Err()
  if err != nil {
    return err
  }

  return nil
}

func (db *RedisDbDriver) DeleteToken(token *model.Token) error {
  err := db.connection.Del(context.Background(), fmt.Sprintf("Tokens:%s", token.Id)).Err()
  if err != nil {
    return err
  }

  err = db.connection.Del(context.Background(), fmt.Sprintf("TokensByUserId:%s:%s", token.UserId, token.Id)).Err()
  if err != nil {
    return err
  }

  return nil
}

func (db *RedisDbDriver) GetToken(id string) (*model.Token, error) {
  var token = &model.Token{}

  v, err := db.connection.Get(context.Background(), fmt.Sprintf("Tokens:%s", id)).Result()
  if err != nil {
    return nil, convertRedisError(err)
  }

  err = json.Unmarshal([]byte(v), &token)
  if err != nil {
    return nil, err
  }

  return token, nil
}

func (db *RedisDbDriver) GetTokensForUser(userId string) ([]*model.Token, error) {
  var tokens []*model.Token

  iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("TokensByUserId:%s:*", userId), 0).Iterator()
  for iter.Next(context.Background()) {
    token, err := db.GetToken(iter.Val()[52:])
    if err != nil {
      return nil, err
    }

    tokens = append(tokens, token)
  }
  if err := iter.Err(); err != nil {
    return nil, err
  }

  return tokens, nil
}
