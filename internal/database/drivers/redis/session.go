package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
)

func (db *RedisDbDriver) SaveSession(session *model.Session) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, session.Id), data, model.SessionExpiryDuration)
	if err != nil {
		return err
	}

	err = db.set(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, session.UserId, session.Id), session.Id, model.SessionExpiryDuration)
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) DeleteSession(session *model.Session) error {
	err := db.del(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, session.Id))
	if err != nil {
		return err
	}

	err = db.del(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, session.UserId, session.Id))
	if err != nil {
		return err
	}

	return nil
}

func (db *RedisDbDriver) GetSession(id string) (*model.Session, error) {
	var session = &model.Session{}

	v, err := db.get(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, id))
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

	prefix := fmt.Sprintf("%sSessionsByUserId:%s:", db.prefix, userId)
	iter := db.scan(context.Background(), prefix+"*")
	for iter.Next(context.Background()) {
		session, err := db.GetSession(strings.TrimPrefix(iter.Val(), prefix))
		if err != nil {
			return nil, err
		}
		if session == nil {
			continue // expired between the scan and the get
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

	iter := db.scan(context.Background(), fmt.Sprintf("%sSessions:*", db.prefix))
	for iter.Next(context.Background()) {
		session, err := db.GetSession(iter.Val()[len(fmt.Sprintf("%sSessions:", db.prefix)):])
		if err != nil {
			return nil, err
		}
		if session == nil {
			continue // expired between the scan and the get
		}

		sessions = append(sessions, session)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	return sessions, nil
}
