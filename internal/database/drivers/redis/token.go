package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveToken(token *model.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sTokens:%s", db.prefix, token.Id), data, model.MaxTokenAge)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sTokensByUserId:%s:%s", db.prefix, token.UserId, token.Id), token.Id, model.MaxTokenAge)
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) DeleteToken(token *model.Token) error {
	err := db.del(context.Background(), fmt.Sprintf("%sTokens:%s", db.prefix, token.Id))
	if err != nil {
		return err
	}

	err = db.del(context.Background(), fmt.Sprintf("%sTokensByUserId:%s:%s", db.prefix, token.UserId, token.Id))
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetToken(id string) (*model.Token, error) {
	var token = &model.Token{}

	v, err := db.get(context.Background(), fmt.Sprintf("%sTokens:%s", db.prefix, id))
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

	prefix := fmt.Sprintf("%sTokensByUserId:%s:", db.prefix, userId)
	iter := db.scan(context.Background(), prefix+"*")
	for iter.Next(context.Background()) {
		token, err := db.GetToken(strings.TrimPrefix(iter.Val(), prefix))
		if err != nil {
			return nil, err
		}
		if token == nil {
			continue // expired between the scan and the get
		}

		tokens = append(tokens, token)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

func (db *RedisDbDriver) GetTokens() ([]*model.Token, error) {
	var tokens []*model.Token

	iter := db.scan(context.Background(), fmt.Sprintf("%sTokens:*", db.prefix))
	for iter.Next(context.Background()) {
		token, err := db.GetToken(iter.Val()[len(fmt.Sprintf("%sTokens:", db.prefix)):])
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
