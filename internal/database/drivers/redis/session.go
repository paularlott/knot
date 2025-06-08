package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveSession(session *model.Session) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, session.Id), data, model.SessionExpiryDuration).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, session.UserId, session.Id), session.Id, model.SessionExpiryDuration).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) DeleteSession(session *model.Session) error {
	err := db.connection.Del(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, session.Id)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, session.UserId, session.Id)).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetSession(id string) (*model.Session, error) {
	var session = &model.Session{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, id)).Result()
	if err != nil {
		return nil, convertRedisError(err)
	}

	err = json.Unmarshal([]byte(v), &session)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (db *RedisDbDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	var sessions []*model.Session

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSessionsByUserId:%s:*", db.prefix, userId), 0).Iterator()
	for iter.Next(context.Background()) {
		session, err := db.GetSession(iter.Val()[len(fmt.Sprintf("%sSessionsByUserId:00000000-0000-0000-0000-000000000000:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}

func (db *RedisDbDriver) GetSessions() ([]*model.Session, error) {
	var sessions []*model.Session

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("%sSessions:*", db.prefix), 0).Iterator()
	for iter.Next(context.Background()) {
		session, err := db.GetSession(iter.Val()[len(fmt.Sprintf("%sSessions:", db.prefix)):])
		if err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}
