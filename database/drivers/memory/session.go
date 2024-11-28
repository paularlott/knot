package driver_memory

import (
	"errors"
	"time"

	"github.com/paularlott/knot/database/model"
)

func (db *MemoryDbDriver) SaveSession(session *model.Session) error {
	// Calculate the expiration time as now + 2 hours
	session.ExpiresAfter = time.Now().UTC().Add(time.Hour * 2)

	db.sessionMutex.Lock()
	defer db.sessionMutex.Unlock()

	db.sessions[session.Id] = session
	db.sessionsByUserId[session.UserId] = append(db.sessionsByUserId[session.UserId], session)

	return nil
}

func (db *MemoryDbDriver) DeleteSession(session *model.Session) error {
	db.sessionMutex.Lock()
	defer db.sessionMutex.Unlock()

	delete(db.sessions, session.Id)
	for i, s := range db.sessionsByUserId[session.UserId] {
		if s.Id == session.Id {
			db.sessionsByUserId[session.UserId] = append(db.sessionsByUserId[session.UserId][:i], db.sessionsByUserId[session.UserId][i+1:]...)
			break
		}
	}

	return nil
}

func (db *MemoryDbDriver) GetSession(id string) (*model.Session, error) {
	db.sessionMutex.RLock()
	defer db.sessionMutex.RUnlock()

	session, ok := db.sessions[id]
	if !ok {
		return nil, errors.New("session not found")
	}

	return session, nil
}

func (db *MemoryDbDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	var sessions []*model.Session

	db.sessionMutex.RLock()
	defer db.sessionMutex.RUnlock()

	for _, session := range db.sessionsByUserId[userId] {
		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (db *MemoryDbDriver) GetSessions() ([]*model.Session, error) {
	var sessions []*model.Session

	db.sessionMutex.RLock()
	defer db.sessionMutex.RUnlock()

	for _, session := range db.sessions {
		sessions = append(sessions, session)
	}

	return sessions, nil
}
