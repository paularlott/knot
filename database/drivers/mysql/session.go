package driver_mysql

import (
	"time"

	"github.com/paularlott/knot/database/model"

	_ "github.com/go-sql-driver/mysql"
)

func (db *MySQLDriver) SaveSession(session *model.Session) error {

	tx, err := db.connection.Begin()
	if err != nil {
		return err
	}

	// Calculate the expiration time as now + 2 hours
	session.ExpiresAfter = time.Now().UTC().Add(time.Hour * 2)

	// Test if the PK exists in the database
	var doUpdate bool
	err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM sessions WHERE session_id=?)", session.Id).Scan(&doUpdate)
	if err != nil {
		tx.Rollback()
		return err
	}

	// Update
	if doUpdate {
		_, err = tx.Exec("UPDATE sessions SET expires_after=? WHERE session_id=?", session.ExpiresAfter.UTC(), session.Id)
	} else {
		_, err = tx.Exec("INSERT INTO sessions (session_id, expires_after, ip, user_id, user_agent, remote_session_id) VALUES (?, ?, ?, ?, ?, ?)", session.Id, session.ExpiresAfter.UTC(), session.Ip, session.UserId, session.UserAgent, session.RemoteSessionId)
	}
	if err != nil {
		tx.Rollback()
		return err
	}

	tx.Commit()

	return nil
}

func (db *MySQLDriver) DeleteSession(session *model.Session) error {
	_, err := db.connection.Exec("DELETE FROM sessions WHERE session_id = ?", session.Id)
	return err
}

func (db *MySQLDriver) getSessions(query string, args ...interface{}) ([]*model.Session, error) {
	var sessions []*model.Session

	rows, err := db.connection.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var session = &model.Session{}
		var expiresAfter string

		err := rows.Scan(&session.Id, &expiresAfter, &session.Ip, &session.UserId, &session.UserAgent, &session.RemoteSessionId)
		if err != nil {
			return nil, err
		}

		// Parse the dates
		session.ExpiresAfter, err = time.Parse("2006-01-02 15:04:05", expiresAfter)
		if err != nil {
			return nil, err
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (db *MySQLDriver) GetSession(id string) (*model.Session, error) {
	sessions, err := db.getSessions("SELECT session_id, expires_after, ip, user_id, user_agent, remote_session_id FROM sessions WHERE session_id = ?", id)
	if err != nil || len(sessions) == 0 {
		return nil, err
	}
	return sessions[0], nil
}

func (db *MySQLDriver) GetSessionsForUser(userId string) ([]*model.Session, error) {
	sessions, err := db.getSessions("SELECT session_id, expires_after, ip, user_id, user_agent, remote_session_id FROM sessions WHERE user_id = ?", userId)
	if err != nil {
		return nil, err
	}

	return sessions, nil
}

func (db *MySQLDriver) GetSessions() ([]*model.Session, error) {
	sessions, err := db.getSessions("SELECT session_id, expires_after, ip, user_id, user_agent, remote_session_id FROM sessions")
	if err != nil {
		return nil, err
	}

	return sessions, nil
}
