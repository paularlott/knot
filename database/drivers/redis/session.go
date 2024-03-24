package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *RedisDbDriver) SaveSession(session *model.Session) error {
	// Calculate the expiration time as now + 2 hours
	session.ExpiresAfter = time.Now().UTC().Add(time.Hour * 2)

	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("Sessions:%s", session.Id), data, time.Hour*2).Err()
	if err != nil {
		return err
	}

	err = db.connection.Set(context.Background(), fmt.Sprintf("SessionsByUserId:%s:%s", session.UserId, session.Id), session.Id, time.Hour*2).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) DeleteSession(session *model.Session) error {
	err := db.connection.Del(context.Background(), fmt.Sprintf("Sessions:%s", session.Id)).Err()
	if err != nil {
		return err
	}

	err = db.connection.Del(context.Background(), fmt.Sprintf("SessionsByUserId:%s:%s", session.UserId, session.Id)).Err()
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetSession(id string) (*model.Session, error) {
	var session = &model.Session{}

	v, err := db.connection.Get(context.Background(), fmt.Sprintf("Sessions:%s", id)).Result()
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

	iter := db.connection.Scan(context.Background(), 0, fmt.Sprintf("SessionsByUserId:%s:*", userId), 0).Iterator()
	for iter.Next(context.Background()) {
		session, err := db.GetSession(iter.Val()[54:])
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

	iter := db.connection.Scan(context.Background(), 0, "Sessions:*", 0).Iterator()
	for iter.Next(context.Background()) {
		session, err := db.GetSession(iter.Val()[9:])
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
