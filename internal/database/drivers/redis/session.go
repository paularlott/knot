package driver_redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/paularlott/knot/internal/database/model"
	"github.com/valkey-io/valkey-go"
)

// saveSessionScript stores a session unless the stored copy is deleted and
// this one isn't, atomically: a deleted session stays deleted, so a request
// that read it before a logout on another server can't save it back to
// life. It returns 1 when it stored the session, and 2 without storing it
// when the stored copy holds a newer provider grant, for the caller to
// carry over and try again.
var saveSessionScript = valkey.NewLuaScript(`
local current = redis.call('GET', KEYS[1])
if current then
  local ok, stored = pcall(cjson.decode, current)
  if ok then
    local ok2, incoming = pcall(cjson.decode, ARGV[1])
    if stored.is_deleted == true and not (ok2 and incoming.is_deleted == true) then
      return 0
    end
    if ok2 and (stored.provider_token_at or 0) > (incoming.provider_token_at or 0) then
      return 2
    end
  end
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
`)

// saveSessionAttempts bounds how often SaveSession carries over a newer
// provider grant saved meanwhile.
const saveSessionAttempts = 3

func (db *RedisDbDriver) SaveSession(session *model.Session) error {
	// A switch rewrites UserId; drop the by-user index left under the
	// previous user or the session lingers in their per-user listings.
	// Only switched sessions can have one, so everyone else skips the
	// extra read on this per-request hot path.
	if session.OriginalUserId != "" {
		if v, err := db.get(context.Background(), fmt.Sprintf("%sSessions:%s", db.prefix, session.Id)); err == nil && v != "" {
			var old model.Session
			if json.Unmarshal([]byte(v), &old) == nil && old.UserId != "" && old.UserId != session.UserId {
				db.del(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, old.UserId, session.Id))
			}
		}
	}

	key := fmt.Sprintf("%sSessions:%s", db.prefix, session.Id)
	toSave := *session
	var saved int64
	for attempt := 0; ; attempt++ {
		data, err := json.Marshal(&toSave)
		if err != nil {
			return err
		}
		saved, err = saveSessionScript.Exec(context.Background(), db.connection,
			[]string{key},
			[]string{string(data), strconv.FormatInt(model.SessionExpiryDuration.Milliseconds(), 10)},
		).AsInt64()
		if err != nil {
			return err
		}
		if saved != 2 {
			break
		}
		if attempt+1 == saveSessionAttempts {
			return fmt.Errorf("session %s: provider grant kept changing while saving", session.Id)
		}
		v, err := db.get(context.Background(), key)
		if err != nil {
			return err
		}
		var stored model.Session
		if err := json.Unmarshal([]byte(v), &stored); err != nil {
			return err
		}
		toSave.KeepNewerProviderToken(&stored)
	}
	if saved == 0 {
		return nil // already deleted, and stays so
	}

	err := db.set(context.Background(), fmt.Sprintf("%sSessionsByUserId:%s:%s", db.prefix, session.UserId, session.Id), session.Id, model.SessionExpiryDuration)
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
